// tynel-toolexec redirects ONLY the handwritten bootstrap package to the
// experimental compiler. Runtime and standard-library packages use stock tools.
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing Go tool")
		os.Exit(2)
	}
	tool, args := os.Args[1], os.Args[2:]
	isCompiler := strings.TrimSuffix(strings.ToLower(filepath.Base(tool)), ".exe") == "compile"
	compiler := os.Getenv("TYNEL_COMPILER")
	input := os.Getenv("TYNEL_IR")
	if isCompiler && compiler != "" && input != "" {
		if len(args) == 1 && args[0] == "-V=full" {
			// go build caches tool identity. Include IR bytes and the compiled
			// bridge, not only a filename, so changed inputs cannot reuse code.
			data, err := os.ReadFile(input)
			if err != nil {
				die(err)
			}
			identity, err := exec.Command(compiler, "-V=full").Output()
			if err != nil {
				die(err)
			}
			line, err := versionLine(string(identity), data, os.Getenv("TYNEL_COMPILER_SHA256"))
			if err != nil {
				die(err)
			}
			fmt.Println(line)
			return
		}
		if os.Getenv("TOOLEXEC_IMPORTPATH") == "tynel.local/bootstrap" {
			tool = compiler
			// Unified-IR export/inlining of synthesized functions is a later
			// milestone. Disable inlining explicitly for this probe package.
			args = append([]string{"-tynel-ir", input, "-l"}, args...)
		}
	}
	cmd := exec.Command(tool, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		die(err)
	}
}

// Go requires the queried tool's canonical name as the first field, even
// when the compiler executable has a different filename.
func versionLine(identity string, input []byte, compilerHash string) (string, error) {
	fields := strings.Fields(identity)
	if len(fields) < 3 || fields[1] != "version" || !strings.HasPrefix(fields[2], "go") {
		return "", fmt.Errorf("unexpected compiler identity %q", identity)
	}
	if len(compilerHash) != 64 {
		return "", fmt.Errorf("missing compiler fingerprint")
	}
	h := sha256.New()
	h.Write(input)
	h.Write([]byte{0})
	h.Write([]byte(compilerHash))
	return fmt.Sprintf("compile %s tynel:%x", strings.Join(fields[1:], " "), h.Sum(nil)), nil
}

func die(err error) { fmt.Fprintln(os.Stderr, "tynel-toolexec:", err); os.Exit(1) }
