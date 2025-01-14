package porto

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	errMainPackage = errors.New("failed to add import to a main package")
)

// addImportPath adds the vanity import path to a given go file.
func addImportPath(absFilepath string, module string) (bool, []byte, error) {
	fset := token.NewFileSet()
	pf, err := parser.ParseFile(fset, absFilepath, nil, 0)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse the file %q: %v", absFilepath, err)
	}
	packageName := pf.Name.String()
	if packageName == "main" { // you can't import a main package
		return false, nil, errMainPackage
	}

	content, err := ioutil.ReadFile(absFilepath)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse the file %q: %v", absFilepath, err)
	}

	// 9 = len("package ") + 1 because that is the first character of the package name
	startPackageLinePos := int(pf.Name.NamePos) - 9

	// first 1 = len(" ") as in "package " and the other 1 is for newline
	endPackageLinePos := pf.Name.NamePos
	newLineChar := byte(10)
	for {
		// we look for new lines in case we already had comments next to the package or
		// another vanity import
		if content[endPackageLinePos] == newLineChar {
			break
		}
		endPackageLinePos++
	}

	importComment := []byte(" // import \"" + module + "\"")

	newContent := []byte{}
	if startPackageLinePos != 0 {
		newContent = append(newContent, content[0:startPackageLinePos]...)
	}
	newContent = append(newContent, []byte("package "+packageName)...)
	newContent = append(newContent, importComment...)
	newContent = append(newContent, content[endPackageLinePos:]...)

	return !bytes.Equal(content, newContent), newContent, nil
}

func isUnexportedModule(moduleName string) bool {
	return strings.Contains(moduleName, "/internal/") ||
		strings.HasSuffix(moduleName, "/internal")
}

func findAndAddVanityImportForModuleDir(workingDir, absDir string, moduleName string, opts Options) (int, error) {
	if isUnexportedModule(moduleName) {
		return 0, nil
	}

	files, err := ioutil.ReadDir(absDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read the content of %q: %v", absDir, err)
	}

	gc := 0
	for _, f := range files {
		if isDir, dirName := f.IsDir(), f.Name(); isDir {
			var (
				c   int
				err error
			)
			if isUnexportedDir(dirName) {
				continue
			} else if newModuleName, ok := findGoModule(absDir + pathSeparator + dirName); ok {
				// if folder contains go.mod we use it from now on to build the vanity import
				c, err = findAndAddVanityImportForModuleDir(workingDir, absDir+pathSeparator+dirName, newModuleName, opts)
				if err != nil {
					return 0, err
				}
			} else {
				// if not, we add the folder name to the vanity import
				if c, err = findAndAddVanityImportForModuleDir(workingDir, absDir+pathSeparator+dirName, moduleName+"/"+dirName, opts); err != nil {
					return 0, err
				}
			}

			gc += c
		} else if fileName := f.Name(); isGoFile(fileName) && !isGoTestFile(fileName) && !isIgnoredFile(opts.SkipFilesRegexes, fileName) {
			absFilepath := absDir + pathSeparator + fileName

			hasChanged, newContent, err := addImportPath(absDir+pathSeparator+fileName, moduleName)
			if !hasChanged {
				continue
			}

			switch err {
			case nil:
				if opts.WriteResultToFile {
					err = writeContentToFile(absFilepath, newContent)
					if err != nil {
						return 0, fmt.Errorf("failed to write file: %v", err)
					}
					gc++
				} else if opts.ListDiffFiles {
					relFilepath, err := filepath.Rel(workingDir, absFilepath)
					if err != nil {
						return 0, fmt.Errorf("failed to resolve relative path: %v", err)
					}
					fmt.Printf("%s: missing right vanity import\n", relFilepath)
					gc++
				} else {
					relFilepath, err := filepath.Rel(workingDir, absFilepath)
					if err != nil {
						return 0, fmt.Errorf("failed to resolve relative path: %v", err)
					}
					fmt.Printf("👉 %s\n\n", relFilepath)
					fmt.Println(string(newContent))
					gc++
				}
			case errMainPackage:
				continue
			default:
				return 0, fmt.Errorf("failed to add vanity import path to %q: %v", absDir+pathSeparator+fileName, err)
			}
		}
	}

	return gc, nil
}

func isIgnoredFile(fileRegexes []*regexp.Regexp, filename string) bool {
	for _, fr := range fileRegexes {
		if matched := fr.MatchString(filename); matched {
			return true
		}
	}

	return false
}

func findAndAddVanityImportForNonModuleDir(workingDir, absDir string, opts Options) (int, error) {
	files, err := ioutil.ReadDir(absDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read %q: %v", absDir, err)
	}

	gc := 0
	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		dirName := f.Name()
		if isUnexportedDir(dirName) {
			continue
		}

		var (
			c   int
			err error
		)

		absDirName := absDir + pathSeparator + dirName
		if moduleName, ok := findGoModule(absDirName); ok {
			if c, err = findAndAddVanityImportForModuleDir(workingDir, dirName, moduleName, opts); err != nil {
				return 0, err
			}
		} else {
			if c, err = findAndAddVanityImportForNonModuleDir(workingDir, absDirName, opts); err != nil {
				return 0, err
			}
		}

		gc += c
	}

	return gc, nil
}

// Options represents the options for adding vanity import.
type Options struct {
	// writes result to file directly
	WriteResultToFile bool
	// List files to be changed
	ListDiffFiles bool
	// Set of regex for matching files to be skipped
	SkipFilesRegexes []*regexp.Regexp
}

// FindAndAddVanityImportForDir scans all files in a folder and based on go.mod files
// encountered decides wether add a vanity import or not.
func FindAndAddVanityImportForDir(workingDir, absDir string, opts Options) (int, error) {
	if moduleName, ok := findGoModule(absDir); ok {
		return findAndAddVanityImportForModuleDir(workingDir, absDir, moduleName, opts)
	}

	files, err := ioutil.ReadDir(absDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read the content of %q: %v", absDir, err)
	}

	gc := 0
	for _, f := range files {
		if !f.IsDir() {
			// we already knew this is not a Go modules folder hence we are not looking
			// for files but for directories
			continue
		}

		dirName := f.Name()
		if isUnexportedDir(dirName) {
			continue
		}

		var (
			c   int
			err error
		)
		absDirName := absDir + pathSeparator + dirName
		if moduleName, ok := findGoModule(absDirName); ok {
			if c, err = findAndAddVanityImportForModuleDir(workingDir, dirName, moduleName, opts); err != nil {
				return 0, err
			}
		} else {
			if c, err = findAndAddVanityImportForNonModuleDir(workingDir, absDirName, opts); err != nil {
				return 0, err
			}
		}

		gc += c
	}

	return gc, nil
}
