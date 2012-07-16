package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	//"runtime"
	"strconv"
	"strings"
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
	// XXX substitutions. Come from lit.cfg in lit.

	script := []string{}
	xfails := []string{}
	xtargets := []string{}
	requires := []string{}
	// Reading a file is lolly long in go :-/
	b, err := ioutil.ReadFile(testfilename)
	if err != nil {
		return nil, err
	}

	//if err != nil { return nil, err }  XXX
	for _, l := range strings.Split(string(b), "\n") {
		//if strings.Contains(l, "RUN:") {
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
		{"%{pathsep}", ":"},
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
	//
	//    isXFail = isExpectedFail(xfails, xtargets, test.suite.config.target_triple)
	//    return script,isXFail,tmpBase,execdir

	//if len(script) > 0 {
	//fmt.Println(script[0])
	//}
}

func executeScript(commands []string, paths *Paths) bool {
	bashPath := "/bin/sh"
	script := paths.tmpBase + ".script" // XXX
	//fmt.Println(script)
	ioutil.WriteFile(script, []byte(strings.Join(commands, " &&\n")), 0666)

	cmd := exec.Command(bashPath, script)

	// XXX: cwd
	// clang/test/lit.cfg: config.test_exec_root = os.path.join(clang_obj_root, 'test')
	cmd.Dir = "/Users/thakis/src/llvm/tools/clang/test"

	// XXX: set env (PATH)
	cmd.Env = []string{
		"PATH=/Users/thakis/src/llvm/Release+Asserts/bin:" + os.Getenv("PATH"),
		/*"LLVM_DISABLE_CRASH_REPORT=1"*/}

	err := cmd.Run()
	if err != nil {
		if werr, ok := err.(*exec.ExitError); ok && !werr.Success() {
			return false
		} else {
			log.Fatal(err)
		}
	}
	return true
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

var total = 0
var fails = 0

func run(testfilename string, index int) {
	state, err := parseIntegratedTestScript(testfilename)
	if err != nil {
		log.Fatal(err)
	}

	if len(state.script) == 0 {
		// XXX lit says "Test has no run line!"?
		return
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

	//  # Create the output directory if it does not already exist.
	//  Util.mkdir_p(os.path.dirname(tmpBase))
	os.MkdirAll(paths.tmpBase, 0777)

	isXFail := isExpectedFail(state.xfails)
	success := executeScript(state.script, paths)
	if !success && !isXFail {
		//fmt.Println("Failed: " + err.Error())
		fmt.Println("Failed: " + testfilename)
		fails++
	} else {
		//fmt.Println("success: " + testfilename)
	}
	total++
}

var i = 0

var c chan int

func walk(path string, info os.FileInfo, err error) error {
	// if filename in ('Output', '.svn') or filename in lc.excludes:
	// lit.cfg adds `config.excludes = ['Inputs']`
	base := filepath.Base(path)
	if contains([]string{".svn", "Output", "Inputs"}, base) {
		return filepath.SkipDir
	}
	// XXX get extension list from config file
	ext := filepath.Ext(base)
	if contains([]string{".c", ".cpp", ".m", ".mm", ".cu", ".ll", ".cl", ".s"}, ext) {
		//fmt.Println(path)
		//go run(path, -1)
		i++
		c <- i
		go func() {
			<-c
			run(path, -1)
		}()

		//run(path)
		//os.Exit(0)
	}
	return nil
}

func main() {
	c = make(chan int, 1) // > 2 -> fd crash

	// XXX needed? system-level stuff uses multiple cores already
	//runtime.GOMAXPROCS(runtime.NumCPU())

	// Find build mode, config files (has path to src root, obj root, tools dir,
	// lib dir, target triple
	// (Also path tweaks on win32)
	// (Also get CLANG env var)

	// Build a list of substitutions from cfg files

	// Parse options

	// Convert @include params

	// Load tests
	//args := [...]string{"/Users/thakis/src/llvm/tools/clang/test"}  // XXX argv

	// tools/clang/test/lit.cfg has:
	//config.test_format = lit.formats.ShTest(execute_external)

	//// suffixes: A list of file extensions to treat as test files.
	//config.suffixes = ['.c', '.cpp', '.m', '.mm', '.cu', '.ll', '.cl', '.s']

	// Build generator for all tests
	// Have test runner that essentially does config.test_format.execute(test, config)
	// sh: TestRunner.executeShTest

	// XXX: subdirectories can contain local configs (e.g. test/Unit/lit.cfg,
	//                                                     test/SemaCXX/Inputs/lit.local.cfg)

	// XXX:

	// Need investigation
	//Failed: /Users/thakis/src/llvm/tools/clang/test/Driver/crash-report.c
	//Failed: /Users/thakis/src/llvm/tools/clang/test/Index/Inputs/crash-recovery-code-complete-remap.c
	filepath.Walk("/Users/thakis/src/llvm/tools/clang/test", walk)

	fmt.Printf("Failed %d / %d", fails, total)
}
