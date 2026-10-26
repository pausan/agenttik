// agenttik_vpx is the whole of libvpx that agenttik calls: open an encoder,
// hand it a BGRA screen, take back one compressed frame. The aop_ calls do the
// same for the screen's sound with libopus. It is linked with a static libvpx
// and libopus into one shared library per platform and loaded from Go
// without cgo (app/internal/screen/vpx). Keeping the surface to plain ints
// and pointers is what makes that safe: libvpx's own API takes large structs
// whose layout changes between versions, and a variadic control call that
// cannot be made portably from Go. See specs/096-screen.md.

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "opus.h"
#include "vpx/vp8cx.h"
#include "vpx/vpx_encoder.h"

#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT __attribute__((visibility("default")))
#endif

typedef struct {
  vpx_codec_ctx_t codec;
  vpx_codec_enc_cfg_t cfg;
  vpx_image_t img;
  int scale; // 1, or 2 to halve each side of the source
  int i444;  // full-resolution colour (VP9 profile 1) rather than 4:2:0
  int src_w, src_h;
  int64_t pts;
  uint8_t *out;
  size_t out_len, out_cap;
} avx;

static const char *last_error = "";

EXPORT const char *avx_error(void) { return last_error; }

// avx_abi numbers the calls below. Go checks it before calling anything, so a
// library built from an older shim is refused rather than called wrongly.
EXPORT int avx_abi(void) { return 3; }

// avx_open starts an encoder for a src_w x src_h screen, encoded at that size
// divided by scale. codec is 8 or 9. crf is the constant quality level and
// kbps its cap; speed is libvpx's cpu-used. i444 keeps colour at full
// resolution, which only VP9 (profile 1) can.
EXPORT avx *avx_open(int codec, int src_w, int src_h, int scale, int threads,
                     int kbps, int crf, int speed, int i444) {
  vpx_codec_iface_t *iface =
      codec == 9 ? vpx_codec_vp9_cx() : vpx_codec_vp8_cx();
  if (scale != 2) scale = 1;
  if (i444 && codec != 9) {
    last_error = "4:4:4 needs VP9";
    return NULL;
  }
  int w = src_w / scale, h = src_h / scale;
  if (w < 16 || h < 16) {
    last_error = "screen too small";
    return NULL;
  }
  vpx_codec_enc_cfg_t cfg;
  if (vpx_codec_enc_config_default(iface, &cfg, 0)) {
    last_error = "no default configuration";
    return NULL;
  }
  cfg.g_w = w;
  cfg.g_h = h;
  cfg.g_profile = i444 ? 1 : 0;
  // Milliseconds: frames come when the screen changes, not on a clock.
  cfg.g_timebase.num = 1;
  cfg.g_timebase.den = 1000;
  cfg.g_threads = threads;
  cfg.g_lag_in_frames = 0;
  cfg.g_error_resilient = 0;
  cfg.rc_end_usage = VPX_CQ;
  cfg.rc_target_bitrate = kbps;
  cfg.rc_min_quantizer = 4;
  cfg.rc_max_quantizer = 56;
  cfg.rc_undershoot_pct = 50;
  cfg.rc_overshoot_pct = 50;
  cfg.rc_buf_sz = 1000;
  cfg.rc_buf_initial_sz = 500;
  cfg.rc_buf_optimal_sz = 600;
  // Keyframes only when asked: at the start and after a viewer resyncs.
  cfg.kf_mode = VPX_KF_DISABLED;

  avx *e = calloc(1, sizeof(avx));
  if (!e) {
    last_error = "out of memory";
    return NULL;
  }
  if (vpx_codec_enc_init(&e->codec, iface, &cfg, 0)) {
    last_error = vpx_codec_error_detail(&e->codec)
                     ? vpx_codec_error_detail(&e->codec)
                     : vpx_codec_error(&e->codec);
    free(e);
    return NULL;
  }
  vpx_codec_control(&e->codec, VP8E_SET_CPUUSED, speed);
  vpx_codec_control(&e->codec, VP8E_SET_CQ_LEVEL, crf);
  if (codec == 9) {
    vpx_codec_control(&e->codec, VP9E_SET_TUNE_CONTENT, VP9E_CONTENT_SCREEN);
    vpx_codec_control(&e->codec, VP9E_SET_AQ_MODE, 3);
    vpx_codec_control(&e->codec, VP9E_SET_ROW_MT, 1);
    vpx_codec_control(&e->codec, VP9E_SET_TILE_COLUMNS, 2);
  } else {
    vpx_codec_control(&e->codec, VP8E_SET_STATIC_THRESHOLD, 100);
  }
  if (!vpx_img_alloc(&e->img, i444 ? VPX_IMG_FMT_I444 : VPX_IMG_FMT_I420, w,
                     h, 32)) {
    vpx_codec_destroy(&e->codec);
    free(e);
    last_error = "out of memory";
    return NULL;
  }
  e->cfg = cfg;
  e->scale = scale;
  e->i444 = i444;
  e->src_w = src_w;
  e->src_h = src_h;
  return e;
}

// BT.601 limited range, the colour space a VP8/VP9 stream that says nothing
// is decoded as.
static inline uint8_t luma(int r, int g, int b) {
  return (uint8_t)(((66 * r + 129 * g + 25 * b + 128) >> 8) + 16);
}
static inline uint8_t chroma_u(int r, int g, int b) {
  return (uint8_t)(((-38 * r - 74 * g + 112 * b + 128) >> 8) + 128);
}
static inline uint8_t chroma_v(int r, int g, int b) {
  return (uint8_t)(((112 * r - 94 * g - 18 * b + 128) >> 8) + 128);
}

// to_i444 converts a BGRA screen with colour for every encoded pixel,
// averaging each scale x scale block.
static void to_i444(avx *e, const uint8_t *bgra, int stride) {
  vpx_image_t *img = &e->img;
  int w = img->d_w, h = img->d_h, s = e->scale;
  for (int y = 0; y < h; y++) {
    const uint8_t *r0 = bgra + (size_t)(s * y) * stride;
    const uint8_t *r1 = r0 + (s == 2 ? stride : 0);
    uint8_t *yo = img->planes[0] + (size_t)y * img->stride[0];
    uint8_t *uo = img->planes[1] + (size_t)y * img->stride[1];
    uint8_t *vo = img->planes[2] + (size_t)y * img->stride[2];
    for (int x = 0; x < w; x++) {
      int bl, gr, rd;
      if (s == 1) {
        const uint8_t *p = r0 + 4 * x;
        bl = p[0], gr = p[1], rd = p[2];
      } else {
        const uint8_t *a = r0 + 8 * x, *b = r1 + 8 * x;
        bl = (a[0] + a[4] + b[0] + b[4] + 2) >> 2;
        gr = (a[1] + a[5] + b[1] + b[5] + 2) >> 2;
        rd = (a[2] + a[6] + b[2] + b[6] + 2) >> 2;
      }
      yo[x] = luma(rd, gr, bl);
      uo[x] = chroma_u(rd, gr, bl);
      vo[x] = chroma_v(rd, gr, bl);
    }
  }
}

// to_i420 converts a BGRA screen into the encoder's image, averaging each
// scale x scale block into one pixel and each 2x2 of those into one chroma
// sample. Plain loops: compilers vectorise them well enough for 1080p in a
// millisecond or two.
static void to_i420(avx *e, const uint8_t *bgra, int stride) {
  vpx_image_t *img = &e->img;
  int w = img->d_w, h = img->d_h, s = e->scale;
  uint8_t *yp = img->planes[0], *up = img->planes[1], *vp = img->planes[2];
  int ys = img->stride[0], us = img->stride[1], vs = img->stride[2];
  if (s == 1) {
    for (int y = 0; y < h; y++) {
      const uint8_t *row = bgra + (size_t)y * stride;
      uint8_t *out = yp + (size_t)y * ys;
      for (int x = 0; x < w; x++)
        out[x] = luma(row[4 * x + 2], row[4 * x + 1], row[4 * x]);
    }
  } else {
    for (int y = 0; y < h; y++) {
      const uint8_t *r0 = bgra + (size_t)(2 * y) * stride;
      const uint8_t *r1 = r0 + stride;
      uint8_t *out = yp + (size_t)y * ys;
      for (int x = 0; x < w; x++) {
        const uint8_t *a = r0 + 8 * x, *b = r1 + 8 * x;
        int bl = (a[0] + a[4] + b[0] + b[4] + 2) >> 2;
        int gr = (a[1] + a[5] + b[1] + b[5] + 2) >> 2;
        int rd = (a[2] + a[6] + b[2] + b[6] + 2) >> 2;
        out[x] = luma(rd, gr, bl);
      }
    }
  }
  // Chroma: one sample per 2x2 encoded pixels, i.e. 2s x 2s source pixels,
  // sampled at the block's four corners' first pixels for speed.
  int cw = (w + 1) / 2, ch = (h + 1) / 2;
  for (int y = 0; y < ch; y++) {
    int sy0 = 2 * y * s, sy1 = sy0 + s;
    if (sy1 >= e->src_h) sy1 = e->src_h - 1;
    const uint8_t *r0 = bgra + (size_t)sy0 * stride;
    const uint8_t *r1 = bgra + (size_t)sy1 * stride;
    uint8_t *uo = up + (size_t)y * us, *vo = vp + (size_t)y * vs;
    for (int x = 0; x < cw; x++) {
      int sx0 = 2 * x * s, sx1 = sx0 + s;
      if (sx1 >= e->src_w) sx1 = e->src_w - 1;
      const uint8_t *a = r0 + 4 * sx0, *b = r0 + 4 * sx1;
      const uint8_t *c = r1 + 4 * sx0, *d = r1 + 4 * sx1;
      int bl = a[0] + b[0] + c[0] + d[0];
      int gr = a[1] + b[1] + c[1] + d[1];
      int rd = a[2] + b[2] + c[2] + d[2];
      uo[x] = (uint8_t)(((-38 * rd - 74 * gr + 112 * bl + 512) >> 10) + 128);
      vo[x] = (uint8_t)(((112 * rd - 94 * gr - 18 * bl + 512) >> 10) + 128);
    }
  }
}

// avx_encode compresses one screen taken ms after the previous one (0 for
// the first). max_q above 0 holds this frame alone to that quantizer
// (0-63): encoding a screen that has stopped changing again this way sharpens
// what motion left blurred. It answers the frame's length, 0 when the
// encoder produced nothing, or -1 on failure; the bytes stay at *out until
// the next call. *key says whether the frame can be decoded on its own.
EXPORT int avx_encode(avx *e, const uint8_t *bgra, int stride, int ms,
                      int force_key, int max_q, uint8_t **out, int *key) {
  if (e->i444)
    to_i444(e, bgra, stride);
  else
    to_i420(e, bgra, stride);
  if (ms < 1) ms = 1;
  vpx_enc_frame_flags_t flags = force_key ? VPX_EFLAG_FORCE_KF : 0;
  int held = max_q > 0 && max_q < (int)e->cfg.rc_max_quantizer;
  if (held) {
    vpx_codec_enc_cfg_t cfg = e->cfg;
    cfg.rc_max_quantizer = max_q;
    if ((int)cfg.rc_min_quantizer > max_q) cfg.rc_min_quantizer = max_q;
    if (vpx_codec_enc_config_set(&e->codec, &cfg)) {
      last_error = vpx_codec_error(&e->codec);
      return -1;
    }
  }
  vpx_codec_err_t err = vpx_codec_encode(&e->codec, &e->img, e->pts, ms,
                                         flags, VPX_DL_REALTIME);
  if (held && vpx_codec_enc_config_set(&e->codec, &e->cfg) && !err)
    err = VPX_CODEC_ERROR;
  if (err) {
    last_error = vpx_codec_error(&e->codec);
    return -1;
  }
  e->pts += ms;
  e->out_len = 0;
  *key = 0;
  vpx_codec_iter_t iter = NULL;
  const vpx_codec_cx_pkt_t *pkt;
  while ((pkt = vpx_codec_get_cx_data(&e->codec, &iter))) {
    if (pkt->kind != VPX_CODEC_CX_FRAME_PKT) continue;
    size_t n = pkt->data.frame.sz;
    if (e->out_len + n > e->out_cap) {
      size_t cap = (e->out_len + n) * 2;
      uint8_t *grown = realloc(e->out, cap);
      if (!grown) {
        last_error = "out of memory";
        return -1;
      }
      e->out = grown;
      e->out_cap = cap;
    }
    memcpy(e->out + e->out_len, pkt->data.frame.buf, n);
    e->out_len += n;
    if (pkt->data.frame.flags & VPX_FRAME_IS_KEY) *key = 1;
  }
  *out = e->out;
  return (int)e->out_len;
}

// avx_set_kbps moves the bitrate cap while encoding, as the viewer's link
// allows more or less. Answers 0 on success.
EXPORT int avx_set_kbps(avx *e, int kbps) {
  if (kbps < 1) kbps = 1;
  if ((int)e->cfg.rc_target_bitrate == kbps) return 0;
  vpx_codec_enc_cfg_t cfg = e->cfg;
  cfg.rc_target_bitrate = kbps;
  if (vpx_codec_enc_config_set(&e->codec, &cfg)) {
    last_error = vpx_codec_error(&e->codec);
    return -1;
  }
  e->cfg = cfg;
  return 0;
}

EXPORT void avx_close(avx *e) {
  if (!e) return;
  vpx_codec_destroy(&e->codec);
  vpx_img_free(&e->img);
  free(e->out);
  free(e);
}

// aop_open starts an Opus encoder for interleaved 16-bit stereo at 48 kHz,
// tuned for music and film rather than speech, variable bitrate around kbps.
EXPORT OpusEncoder *aop_open(int kbps) {
  int err;
  OpusEncoder *e = opus_encoder_create(48000, 2, OPUS_APPLICATION_AUDIO, &err);
  if (err != OPUS_OK) {
    last_error = opus_strerror(err);
    return NULL;
  }
  opus_encoder_ctl(e, OPUS_SET_BITRATE(kbps * 1000));
  opus_encoder_ctl(e, OPUS_SET_SIGNAL(OPUS_SIGNAL_MUSIC));
  return e;
}

// aop_encode compresses one frame of samples per channel (960 is 20 ms) into
// out, answering the packet's length or -1.
EXPORT int aop_encode(OpusEncoder *e, const int16_t *pcm, int samples,
                      uint8_t *out, int cap) {
  int n = opus_encode(e, pcm, samples, out, cap);
  if (n < 0) {
    last_error = opus_strerror(n);
    return -1;
  }
  return n;
}

EXPORT void aop_close(OpusEncoder *e) { opus_encoder_destroy(e); }
