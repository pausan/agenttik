// Package attachments keeps pasted images outside project working trees.
package attachments

import (
	"fmt"
	"path/filepath"
	"regexp"
)

var Name = regexp.MustCompile(`^[a-f0-9]{64}\.(png|jpg|gif|webp)$`)
var reference = regexp.MustCompile(`!\[Attached image\]\(/api/attachments/([a-f0-9]{64}\.(?:png|jpg|gif|webp))\)`)

// Resolve gives local agents a file they can open with their image-reading
// tool. Stored prompts keep portable HTTP references for the UI and queue.
func Resolve(prompt, dir string) string {
	return reference.ReplaceAllStringFunc(prompt, func(s string) string {
		name := reference.FindStringSubmatch(s)[1]
		path, _ := filepath.Abs(filepath.Join(dir, "attachments", name))
		return fmt.Sprintf("Read the attached image using your image-reading tool: %q", path)
	})
}
