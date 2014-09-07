// run

// +build !akaros,!nacl

// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var tests = `
# These are test cases for the linker analysis that detects chains of
# nosplit functions that would cause a stack overflow.
#
# Lines beginning with # are comments.
#
# Each test case describes a sequence of functions, one per line.
# Each function definition is the function name, then the frame size,
# then optionally the keyword 'nosplit', then the body of the function.
# The body is assembly code, with some shorthands.
# The shorthand 'call x' stands for CALL x(SB).
# The shorthand 'callind' stands for 'CALL R0', where R0 is a register.
# Each test case must define a function named main, and it must be first.
# That is, a line beginning "main " indicates the start of a new test case.
# Within a stanza, ; can be used instead of \n to separate lines.
#
# After the function definition, the test case ends with an optional
# REJECT line, specifying the architectures on which the case should
# be rejected. "REJECT" without any architectures means reject on all architectures.
# The linker should accept the test case on systems not explicitly rejected.
#
# 64-bit systems do not attempt to execute test cases with frame sizes
# that are only 32-bit aligned.

# Ordinary function should work
main 0

# Large frame marked nosplit is always wrong.
main 10000 nosplit
REJECT

# Calling a large frame is okay.
main 0 call big
big 10000

# But not if the frame is nosplit.
main 0 call big
big 10000 nosplit
REJECT

# Recursion is okay.
main 0 call main

# Recursive nosplit runs out of space.
main 0 nosplit call main
REJECT

# Chains of ordinary functions okay.
main 0 call f1
f1 80 call f2
f2 80

# Chains of nosplit must fit in the stack limit, 128 bytes.
main 0 call f1
f1 80 nosplit call f2
f2 80 nosplit
REJECT

# Larger chains.
main 0 call f1
f1 16 call f2
f2 16 call f3
f3 16 call f4
f4 16 call f5
f5 16 call f6
f6 16 call f7
f7 16 call f8
f8 16 call end
end 1000

main 0 call f1
f1 16 nosplit call f2
f2 16 nosplit call f3
f3 16 nosplit call f4
f4 16 nosplit call f5
f5 16 nosplit call f6
f6 16 nosplit call f7
f7 16 nosplit call f8
f8 16 nosplit call end
end 1000
REJECT

# Test cases near the 128-byte limit.

# Ordinary stack split frame is always okay.
main 112
main 116
main 120
main 124
main 128
main 132
main 136

# A nosplit leaf can use the whole 128-CallSize bytes available on entry.
main 112 nosplit
main 116 nosplit
main 120 nosplit
main 124 nosplit
main 128 nosplit; REJECT
main 132 nosplit; REJECT
main 136 nosplit; REJECT

# Calling a nosplit function from a nosplit function requires
# having room for the saved caller PC and the called frame.
# Because ARM doesn't save LR in the leaf, it gets an extra 4 bytes.
main 112 nosplit call f; f 0 nosplit
main 116 nosplit call f; f 0 nosplit; REJECT amd64
main 120 nosplit call f; f 0 nosplit; REJECT amd64
main 124 nosplit call f; f 0 nosplit; REJECT amd64 386
main 128 nosplit call f; f 0 nosplit; REJECT
main 132 nosplit call f; f 0 nosplit; REJECT
main 136 nosplit call f; f 0 nosplit; REJECT

# Calling a splitting function from a nosplit function requires
# having room for the saved caller PC of the call but also the
# saved caller PC for the call to morestack. Again the ARM works
# in less space.
main 104 nosplit call f; f 0 call f
main 108 nosplit call f; f 0 call f
main 112 nosplit call f; f 0 call f; REJECT amd64
main 116 nosplit call f; f 0 call f; REJECT amd64
main 120 nosplit call f; f 0 call f; REJECT amd64 386
main 124 nosplit call f; f 0 call f; REJECT amd64 386
main 128 nosplit call f; f 0 call f; REJECT
main 132 nosplit call f; f 0 call f; REJECT
main 136 nosplit call f; f 0 call f; REJECT

# Indirect calls are assumed to be splitting functions.
main 104 nosplit callind
main 108 nosplit callind
main 112 nosplit callind; REJECT amd64
main 116 nosplit callind; REJECT amd64
main 120 nosplit callind; REJECT amd64 386
main 124 nosplit callind; REJECT amd64 386
main 128 nosplit callind; REJECT
main 132 nosplit callind; REJECT
main 136 nosplit callind; REJECT

# Issue 7623
main 0 call f; f 112
main 0 call f; f 116
main 0 call f; f 120
main 0 call f; f 124
main 0 call f; f 128
main 0 call f; f 132
main 0 call f; f 136
`

var (
	commentRE = regexp.MustCompile(`(?m)^#.*`)
	rejectRE  = regexp.MustCompile(`(?s)\A(.+?)((\n|; *)REJECT(.*))?\z`)
	lineRE    = regexp.MustCompile(`(\w+) (\d+)( nosplit)?(.*)`)
	callRE    = regexp.MustCompile(`\bcall (\w+)\b`)
	callindRE = regexp.MustCompile(`\bcallind\b`)
)

func main() {
	goarch := os.Getenv("GOARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	dir, err := ioutil.TempDir("", "go-test-nosplit")
	if err != nil {
		bug()
		fmt.Printf("creating temp dir: %v\n", err)
		return
	}
	defer os.RemoveAll(dir)
	ioutil.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main()\n"), 0666)

	tests = strings.Replace(tests, "\t", " ", -1)
	tests = commentRE.ReplaceAllString(tests, "")

	nok := 0
	nfail := 0
TestCases:
	for len(tests) > 0 {
		var stanza string
		i := strings.Index(tests, "\nmain ")
		if i < 0 {
			stanza, tests = tests, ""
		} else {
			stanza, tests = tests[:i], tests[i+1:]
		}

		m := rejectRE.FindStringSubmatch(stanza)
		if m == nil {
			bug()
			fmt.Printf("invalid stanza:\n\t%s\n", indent(stanza))
			continue
		}
		lines := strings.TrimSpace(m[1])
		reject := false
		if m[2] != "" {
			if strings.TrimSpace(m[4]) == "" {
				reject = true
			} else {
				for _, rej := range strings.Fields(m[4]) {
					if rej == goarch {
						reject = true
					}
				}
			}
		}
		if lines == "" && !reject {
			continue
		}

		var buf bytes.Buffer
		if goarch == "arm" {
			fmt.Fprintf(&buf, "#define CALL BL\n#define REGISTER (R0)\n")
		} else {
			fmt.Fprintf(&buf, "#define REGISTER AX\n")
		}

		for _, line := range strings.Split(lines, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			for i, subline := range strings.Split(line, ";") {
				subline = strings.TrimSpace(subline)
				if subline == "" {
					continue
				}
				m := lineRE.FindStringSubmatch(subline)
				if m == nil {
					bug()
					fmt.Printf("invalid function line: %s\n", subline)
					continue TestCases
				}
				name := m[1]
				size, _ := strconv.Atoi(m[2])

				// The limit was originally 128 but is now 384.
				// Instead of rewriting the test cases above, adjust
				// the first stack frame to use up the extra 32 bytes.
				if i == 0 {
					size += 384 - 128
				}

				if goarch == "amd64" && size%8 == 4 {
					continue TestCases
				}
				nosplit := m[3]
				body := m[4]

				if nosplit != "" {
					nosplit = ",7"
				} else {
					nosplit = ",0"
				}
				body = callRE.ReplaceAllString(body, "CALL ·$1(SB);")
				body = callindRE.ReplaceAllString(body, "CALL REGISTER;")

				fmt.Fprintf(&buf, "TEXT ·%s(SB)%s,$%d-0\n\t%s\n\tRET\n\n", name, nosplit, size, body)
			}
		}

		ioutil.WriteFile(filepath.Join(dir, "asm.s"), buf.Bytes(), 0666)

		cmd := exec.Command("go", "build")
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err == nil {
			nok++
			if reject {
				bug()
				fmt.Printf("accepted incorrectly:\n\t%s\n", indent(strings.TrimSpace(stanza)))
			}
		} else {
			nfail++
			if !reject {
				bug()
				fmt.Printf("rejected incorrectly:\n\t%s\n", indent(strings.TrimSpace(stanza)))
				fmt.Printf("\n\tlinker output:\n\t%s\n", indent(string(output)))
			}
		}
	}

	if !bugged && (nok == 0 || nfail == 0) {
		bug()
		fmt.Printf("not enough test cases run\n")
	}
}

func indent(s string) string {
	return strings.Replace(s, "\n", "\n\t", -1)
}

var bugged = false

func bug() {
	if !bugged {
		bugged = true
		fmt.Printf("BUG\n")
	}
}
