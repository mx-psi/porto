package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jcchavezs/porto"
)

func main() {
	flagWriteOutputToFile := flag.Bool("w", false, "write result to (source) file instead of stdout")
	flagListDiff := flag.Bool("l", false, "list files whose vanity import differs from porto's")
	flagSkipFiles := flag.String("skip-files", "", "Regexps of files to skip")
	flag.Parse()

	baseDir := flag.Arg(0)
	if len(flag.Args()) == 0 {
		fmt.Println(`
usage: porto [options] <target-path>

Options:
-w             Write result to (source) file instead of stdout (default: false)
-l             List files whose vanity import differs from porto's (default: false)
--skip-files   Regexps of files to skip

Examples:

Add import path to a folder
    $ porto -w ./myproject
		`)
		os.Exit(0)
	}

	baseAbsDir, err := filepath.Abs(baseDir)
	if err != nil {
		log.Fatalf("failed to resolve base absolute path for target path %q: %v", baseDir, err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to resolve base absolute path for current working dir: %v", err)
	}

	var skipFilesRegex []*regexp.Regexp
	if len(*flagSkipFiles) > 0 {
		for _, sfrp := range strings.Split(*flagSkipFiles, ",") {
			sfr, err := regexp.Compile(sfrp)
			if err != nil {
				log.Fatalf("failed to resolve base absolute path for %q: %v", baseDir, err)
			}
			skipFilesRegex = append(skipFilesRegex, sfr)
		}
	}

	diffCount, err := porto.FindAndAddVanityImportForDir(workingDir, baseAbsDir, porto.Options{
		WriteResultToFile: *flagWriteOutputToFile,
		ListDiffFiles:     *flagListDiff,
		SkipFilesRegexes:  skipFilesRegex,
	})
	if err != nil {
		log.Fatal(err)
	}

	if *flagListDiff && diffCount > 0 {
		os.Exit(2)
	}
}
