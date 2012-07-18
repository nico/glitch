package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func contains(strings []string, s string) bool {
	for _, v := range strings {
		if v == s {
			return true
		}
	}
	return false
}

type TestScript struct {
	script   []string
	xfails   []string
	xtargets []string
	requires []string
}

// Scan an LLVM/Clang style integrated test script and extract the lines to
// 'RUN' as well as 'XFAIL' and 'XTARGET' information. The RUN lines also will
// have variable substitution performed.
func parseIntegratedTestScript(testfilename string) (*TestScript, error) {
	script := []string{}
	xfails := []string{}
	xtargets := []string{}
	requires := []string{}
	// Reading a file is lolly long in go :-/
	b, err := ioutil.ReadFile(testfilename)
	if err != nil {
		return nil, err
	}

	for _, l := range strings.Split(string(b), "\n") {
		if index := strings.Index(l, "RUN:"); index != -1 {
			// Isolate the command to run.
			l = l[index+len("RUN:"):]

			// Trim trailing whitespace.
			//l = strings.TrimRight(l)
			l = strings.TrimSpace(l)

			// Collapse lines with trailing '\\'.
			// XXX no arr[-1]? :-/
			if len(script) > 0 && script[len(script)-1][len(script[len(script)-1])-1] == '\\' {
				script[len(script)-1] = script[len(script)-1][:len(script[len(script)-1])-1] + l
			} else {
				script = append(script, l)
			}
		} else if index := strings.Index(l, "XFAIL:"); index != -1 {
			items := strings.Split(l[index+len("XFAIL:"):], ",")
			for i := range items { // XXX list comprehensions / map() ?
				items[i] = strings.TrimSpace(items[i])
			}
			xfails = append(xfails, items...)
		} else if index := strings.Index(l, "XTARGET:"); index != -1 {
			items := strings.Split(l[index+len("XTARGET:"):], ",")
			for i := range items { // XXX list comprehensions / map() ?
				items[i] = strings.TrimSpace(items[i])
			}
			xtargets = append(xtargets, items...)
		} else if index := strings.Index(l, "REQUIRES:"); index != -1 {
			items := strings.Split(l[index+len("REQUIRES:"):], ",")
			for i := range items { // XXX list comprehensions / map() ?
				items[i] = strings.TrimSpace(items[i])
			}
			requires = append(requires, items...)
		} else if index := strings.Index(l, "END."); index != -1 {
			if strings.TrimSpace(l[index+len("END."):]) == "END." {
				break
			}
		}
	}
	return &TestScript{script, xfails, xtargets, requires}, nil
}

type Paths struct {
	sourcepath string
	sourcedir  string
	execbase   string
	execdir    string
	tmpDir     string
	tmpBase    string
}

func dosubst(script []string, paths *Paths) {
	// Apply substitutions to the script.  Allow full regular
	// expression syntax.  Replace each matching occurrence of regular
	// expression pattern a with substitution b in line ln.

	clang := "/Users/thakis/src/llvm/Release+Asserts/bin/clang" // XXX
	subst := [][]string{
		{"%%", "#_MARKER_#"},

		// XXX -print-file-name=include
		{"%clang_cc1", clang + " -cc1 -internal-isystem /Users/thakis/src/llvm/Release+Asserts/bin/../lib/clang/3.2/include"},
		{"%clangxx", clang + " -ccc-clang-cxx -ccc-cxx"},
		{"%clang", " " + clang + " "},
		{"%test_debuginfo", " /Users/thakis/src/llvm/utils/test_debuginfo.pl "}, // XXX
		// XXX prohobit clang, clang-cc, clang -cc1 for %clang, %clang_cc1

		{"%s", paths.sourcepath},
		{"%S", paths.sourcedir},
		{"%p", paths.sourcedir},
		{"%{pathsep}", string(os.PathListSeparator)},
		{"%t", paths.tmpBase + ".tmp"},
		{"%T", paths.tmpDir},
		{"#_MARKER_#", "%"},
	}
	// Apply substitutions
	for i := range script {
		for _, s := range subst {
			// XXX lit uses re.sub
			script[i] = strings.Replace(script[i], s[0], s[1], -1)
		}
	}
	//    # Verify the script contains a run line.
	//    if not script:
	//        return (Test.UNRESOLVED, "Test has no run line!")
	//
	//    # Check for unterminated run lines.
	//    if script[-1][-1] == '\\':
	//        return (Test.UNRESOLVED, "Test has unterminated run lines (with '\\')")
	//
	//    # Check that we have the required features:
	//    missing_required_features = [f for f in requires
	//                                 if f not in test.config.available_features]
	//    if missing_required_features:
	//        msg = ', '.join(missing_required_features)
	//        return (Test.UNSUPPORTED,
	//                "Test requires the following features: %s" % msg)
}

func executeScript(commands []string, paths *Paths) (bool, *bytes.Buffer, *bytes.Buffer) {
	// XXX: better bash lookup
	bashPath := "/bin/bash" // NOTE: '/bin/sh' makes Driver/crash-report.c fail
	script := paths.tmpBase + ".script"
	//fmt.Println(script)
	ioutil.WriteFile(script, []byte(strings.Join(commands, " &&\n")), 0666)

	cmd := exec.Command(bashPath, script)

	// XXX: cwd
	// clang/test/lit.cfg: config.test_exec_root = os.path.join(clang_obj_root, 'test')
	cmd.Dir = "/Users/thakis/src/llvm/tools/clang/test"

	// XXX: set env (PATH)
	cmd.Env = []string{
		"PATH=/Users/thakis/src/llvm/Release+Asserts/bin:" + os.Getenv("PATH")}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if werr, ok := err.(*exec.ExitError); ok && !werr.Success() {
			return false, &stdout, &stderr
		} else {
			log.Fatal(err)
		}
	}
	return true, &stdout, &stderr
}

func isExpectedFail(xfails []string) bool {
	for _, item := range xfails {
		if item == "*" { // XXX look at target triple
			return true
		}
	}
	// XXX look at xtarget too
	return false
}

func run(testfilename string, index int) TestResult {
	state, err := parseIntegratedTestScript(testfilename)
	if err != nil {
		log.Fatal(err)
	}

	if len(state.script) == 0 {
		// XXX lit says "Test has no run line!"
		// XXX this currently happens for .cpp files below unittests
		return TestResult{true, "", ""}
	}

	paths := &Paths{}
	paths.sourcepath = testfilename
	//paths.sourcedir = filepath.Dir(sourcepath)
	paths.sourcedir = filepath.Dir(paths.sourcepath)
	//paths.execpath = sourcepath
	paths.execbase = filepath.Base(paths.sourcepath)
	paths.execdir = filepath.Dir(paths.sourcepath)
	paths.tmpDir = filepath.Join(paths.execdir, "Output")
	paths.tmpBase = filepath.Join(paths.tmpDir, paths.execbase)
	if index >= 0 {
		paths.tmpBase += "_" + strconv.Itoa(index)
	}

	dosubst(state.script, paths)

	// Create the output directory if it does not already exist.
	os.MkdirAll(paths.tmpBase, 0777)

	isXFail := isExpectedFail(state.xfails)
	success, stdout, stderr := executeScript(state.script, paths)
	if !success && !isXFail {
		return TestResult{false, stdout.String(), stderr.String()}
	}
	return TestResult{true, stdout.String(), stderr.String()}
}

func executegtest(testexe string, testname string) TestResult {
	cmd := exec.Command(testexe, "--gtest_filter="+testname)
	// XXX env / pwd?

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if werr, ok := err.(*exec.ExitError); ok && !werr.Success() {
			return TestResult{false, stdout.String(), stderr.String()}
		} else {
			log.Fatal(err)
		}
	}
	return TestResult{true, stdout.String(), stderr.String()}
}

func getGTestTests(path string) []string {
	cmd := exec.Command(path, "--gtest_list_tests")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	result := []string{}
	currentTestClass := ""
	for _, l := range strings.Split(string(stdout.String()), "\n") {
		if !strings.HasPrefix(l, "  ") {
			currentTestClass = l
			continue
		}
		if l[2] == ' ' {
			log.Fatal(l)
		}
		result = append(result, currentTestClass+l[2:])
	}
	return result
}

var i = 0
var maxdone = 0

type Test struct {
	name string
	run  func() TestResult
}

type TestResult struct {
	success bool
	stdout  string
	stderr  string
}

func findTests(path string, result []*Test) []*Test {
	// XXX get extension list from config file?
	ext := filepath.Ext(path)
	if contains([]string{".c", ".cpp", ".m", ".mm", ".cu", ".ll", ".cl", ".s"}, ext) {
		result = append(result, &Test{filepath.Base(path), func() TestResult { return run(path, -1) }})
	} else if ext == "" && strings.HasSuffix(path, "Tests") {
		for _, name := range getGTestTests(path) {
			result = append(result, &Test{
				name,
				func() TestResult {
					return executegtest(path, name)
				}})
		}
	}
	return result
}

func main() {
	flag.Parse()

	// XXX needed? system-level stuff uses multiple cores already
	//runtime.GOMAXPROCS(runtime.NumCPU())

	// XXX get clang binary dir from flag

	// XXX: Why does this have a RUN: line?
	//Failed: /Users/thakis/src/llvm/tools/clang/test/Index/Inputs/crash-recovery-code-complete-remap.c

	// XXX: try to pass a something to Walk that can access parameters, use that to inject paths
	// (or let Walk just do the generator thing?)
	tests := []*Test{}
	walk := func(path string, info os.FileInfo, err error) error {
		// if filename in ('Output', '.svn') or filename in lc.excludes:
		// lit.cfg adds `config.excludes = ['Inputs']`
		if contains([]string{".svn", "Output", "Inputs"}, filepath.Base(path)) {
			return filepath.SkipDir
		}
		// If there's a command-line arg, filter tests on it
		if len(flag.Args()) > 0 && !strings.Contains(path, flag.Arg(0)) {
			return nil
		}
		tests = findTests(path, tests)
		return nil
	}
	filepath.Walk("/Users/thakis/src/llvm/tools/clang/test", walk)
	filepath.Walk("/Users/thakis/src/llvm/tools/clang/unittests", walk)

	var total = 0
	var fails = 0
	c := make(chan int, runtime.NumCPU())
	for _, test := range tests {
		// Don't start more jobs than cap(c) at once
		i++
		c <- i
		go func() {
			result := test.run()
			if !result.success {
				fmt.Println("Failed: " + test.name)
				fmt.Println("stdout: ", result.stdout)
				fmt.Println("stderr: ", result.stderr)
				fails++
			}
			total++
			maxdone++
			<-c
		}()
	}

	// XXX: fancy progress meter
	// XXX: slightly more detailed status / error printing

	// Wait for all tests to complete!
	for maxdone < i {
		time.Sleep(1e9 / 4) // 1/4s
	}

	fmt.Printf("Failed %d / %d", fails, total)
}
