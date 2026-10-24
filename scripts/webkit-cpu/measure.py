"""Opens the UI in WebKitGTK with hardware acceleration off, as the Linux
desktop window has it, runs each script two seconds apart, and prints the CPU
the web process and the window's own process used over the next 20 seconds.

usage: measure.py <url> <label> [script...]"""
import os
import sys
import time

import gi

gi.require_version("Gtk", "3.0")
gi.require_version("WebKit2", "4.1")
from gi.repository import GLib, Gtk, WebKit2  # noqa: E402

url, label, scripts = sys.argv[1], sys.argv[2], sys.argv[3:]
window = Gtk.Window()
window.set_default_size(1600, 1000)
view = WebKit2.WebView()
view.get_settings().set_hardware_acceleration_policy(WebKit2.HardwareAccelerationPolicy.NEVER)
window.add(view)
window.show_all()
view.load_uri(url)


def ticks(pid):
    fields = open(f"/proc/{pid}/stat").read().rsplit(")", 1)[1].split()
    return int(fields[11]) + int(fields[12])


def children():
    me, found = os.getpid(), []
    for pid in filter(str.isdigit, os.listdir("/proc")):
        try:
            if int(open(f"/proc/{pid}/stat").read().rsplit(")", 1)[1].split()[1]) == me:
                found.append(pid)
        except OSError:
            pass
    return found


start = {}


def run(script):
    view.evaluate_javascript(script, -1, None, None, None, None, None)
    return False


def begin():
    start["pids"] = children()
    start["web"] = sum(ticks(p) for p in start["pids"])
    start["ui"] = ticks("self")
    start["at"] = time.time()
    return False


def end():
    hz, took = os.sysconf("SC_CLK_TCK"), time.time() - start["at"]
    web = 100 * (sum(ticks(p) for p in start["pids"]) - start["web"]) / hz / took
    ui = 100 * (ticks("self") - start["ui"]) / hz / took
    print(f"{label:30} web process {web:5.1f}%  window {ui:5.1f}%", flush=True)
    Gtk.main_quit()
    return False


for i, script in enumerate(scripts):
    GLib.timeout_add(5000 + 2000 * i, run, script)
GLib.timeout_add(10000, begin)
GLib.timeout_add(30000, end)
Gtk.main()
