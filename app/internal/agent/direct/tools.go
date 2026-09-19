package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/process"
)

func codingTools(permission agent.Permission, env environment) []any {
	tool := func(name, description string, properties map[string]any, required ...string) any {
		return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "parameters": map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}}
	}
	str := map[string]string{"type": "string"}
	out := []any{
		tool("read_file", "Read a project file (up to 64 KiB).", map[string]any{"path": str}, "path"),
		tool("list_directory", "List a project directory. Use . for the project root.", map[string]any{"path": str}, "path"),
	}
	if permission.Valid() != agent.PermissionPlan {
		out = append(out, tool("write_file", "Write a UTF-8 project file. Missing parent directories are created.", map[string]any{"path": str, "content": str}, "path", "content"))
	}
	if permission.Valid() == agent.PermissionFull && len(env.Shells) > 0 {
		out = append(out, tool("shell", "Run a shell command in the project. Timeout: 60 seconds.", map[string]any{"command": str, "shell": str}, "command"))
	}
	if permission.Valid() == agent.PermissionFull && env.Python != "" {
		out = append(out, tool("python", "Run Python code in the project. Timeout: 60 seconds.", map[string]any{"code": str}, "code"))
	}
	return out
}
func RunTool(ctx context.Context, req agent.TurnRequest, call ToolCall) (string, error) {
	return runTool(ctx, req, call, detectEnvironment())
}

func runTool(ctx context.Context, req agent.TurnRequest, call ToolCall, env environment) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Command string `json:"command"`
		Shell   string `json:"shell"`
		Code    string `json:"code"`
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil {
		return "", fmt.Errorf("invalid tool arguments")
	}
	if call.Function.Name == "shell" || call.Function.Name == "python" {
		if req.Permission.Valid() != agent.PermissionFull {
			return "", fmt.Errorf("shell and Python require full access")
		}
		shellCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		var cmd *exec.Cmd
		if call.Function.Name == "python" {
			if env.Python == "" {
				return "", fmt.Errorf("Python is not available")
			}
			cmd = exec.CommandContext(shellCtx, env.Python, "-c", args.Code)
		} else {
			shell, ok := env.shell(args.Shell)
			if !ok {
				return "", fmt.Errorf("requested shell is not available")
			}
			argv := append(append([]string{}, shell.Args...), args.Command)
			cmd = exec.CommandContext(shellCtx, shell.Path, argv...)
		}
		cmd.Dir = req.WorkDir
		process.Configure(cmd)
		cmd.Cancel = func() error { return process.Kill(cmd.Process) }
		cmd.WaitDelay = time.Second
		output := &LimitedOutput{}
		cmd.Stdout = output
		cmd.Stderr = output
		err := cmd.Run()
		if err != nil {
			return "", fmt.Errorf("%s\n%v", output.String(), err)
		}
		return output.String(), nil
	}
	dir := req.WorkDir
	if dir == "" {
		return "", fmt.Errorf("a project folder is required for file tools")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	switch call.Function.Name {
	case "read_file":
		f, err := root.Open(args.Path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("only regular files can be read")
		}
		b, err := io.ReadAll(io.LimitReader(f, 65537))
		if len(b) > 65536 {
			return string(b[:65536]) + "\n[file truncated at 64 KiB]", err
		}
		return string(b), err
	case "list_directory":
		f, err := root.Open(args.Path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		entries, err := f.ReadDir(1000)
		if err != nil && err != io.EOF {
			return "", err
		}
		var b strings.Builder
		for _, entry := range entries {
			b.WriteString(entry.Name())
			if entry.IsDir() {
				b.WriteByte('/')
			}
			b.WriteByte('\n')
		}
		if len(entries) == 1000 {
			b.WriteString("[limited to 1000 entries]\n")
		}
		return b.String(), nil
	case "write_file":
		if req.Permission.Valid() == agent.PermissionPlan {
			return "", fmt.Errorf("plan mode cannot write files")
		}
		if len(args.Content) > 1<<20 {
			return "", fmt.Errorf("file exceeds 1 MiB")
		}
		if info, err := root.Stat(args.Path); err == nil && !info.Mode().IsRegular() {
			return "", fmt.Errorf("only regular files can be written")
		}
		if err := root.MkdirAll(filepath.Dir(args.Path), 0755); err != nil {
			return "", err
		}
		err := root.WriteFile(args.Path, []byte(args.Content), 0644)
		return "File written.", err
	default:
		return "", fmt.Errorf("unknown tool %q", call.Function.Name)
	}
}

type LimitedOutput struct{ strings.Builder }

func (b *LimitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if left := 65536 - b.Len(); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		_, _ = b.Builder.Write(p)
	}
	return n, nil
}
