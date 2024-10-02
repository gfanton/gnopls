// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

//go:generate go run ./copier.go

// Copier is a tool to automate copy of govulncheck's internal files.
//
//   - copy golang.org/x/vuln/internal/osv/ to osv
//   - copy golang.org/x/vuln/internal/govulncheck/ to govulncheck
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gfanton/gnopls/internal/edit"
)

func main() {
	log.SetPrefix("copier: ")
	log.SetFlags(log.Lshortfile)

	srcMod := "golang.org/x/vuln"
	srcModVers := "@latest"
	srcDir, srcVer := downloadModule(srcMod + srcModVers)

	cfg := rewrite{
		banner:        fmt.Sprintf("// Code generated by copying from %v@%v (go run copier.go); DO NOT EDIT.", srcMod, srcVer),
		srcImportPath: "golang.org/x/vuln/internal",
		dstImportPath: currentPackagePath(),
	}

	copyFiles("osv", filepath.Join(srcDir, "internal", "osv"), cfg)
	copyFiles("govulncheck", filepath.Join(srcDir, "internal", "govulncheck"), cfg)
}

type rewrite struct {
	// DO NOT EDIT marker to add at the beginning
	banner string
	// rewrite srcImportPath with dstImportPath
	srcImportPath string
	dstImportPath string
}

func copyFiles(dst, src string, cfg rewrite) {
	entries, err := os.ReadDir(src)
	if err != nil {
		log.Fatalf("failed to read dir: %v", err)
	}
	if err := os.MkdirAll(dst, 0777); err != nil {
		log.Fatalf("failed to create dir: %v", err)
	}

	for _, e := range entries {
		fname := e.Name()
		// we need only non-test go files.
		if e.IsDir() || !strings.HasSuffix(fname, ".go") || strings.HasSuffix(fname, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, fname))
		if err != nil {
			log.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, fname, data, parser.ParseComments|parser.ImportsOnly)
		if err != nil {
			log.Fatalf("parsing source module:\n%s", err)
		}

		buf := edit.NewBuffer(data)
		at := func(p token.Pos) int {
			return fset.File(p).Offset(p)
		}

		// Add banner right after the copyright statement (the first comment)
		bannerInsert, banner := f.FileStart, cfg.banner
		if len(f.Comments) > 0 && strings.HasPrefix(f.Comments[0].Text(), "Copyright ") {
			bannerInsert = f.Comments[0].End()
			banner = "\n\n" + banner
		}
		buf.Replace(at(bannerInsert), at(bannerInsert), banner)

		// Adjust imports
		for _, spec := range f.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				log.Fatal(err)
			}
			if strings.HasPrefix(path, cfg.srcImportPath) {
				newPath := strings.Replace(path, cfg.srcImportPath, cfg.dstImportPath, 1)
				buf.Replace(at(spec.Path.Pos()), at(spec.Path.End()), strconv.Quote(newPath))
			}
		}
		data = buf.Bytes()

		if err := os.WriteFile(filepath.Join(dst, fname), data, 0666); err != nil {
			log.Fatal(err)
		}
	}
}

func downloadModule(srcModVers string) (dir, ver string) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "mod", "download", "-json", srcModVers)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("go mod download -json %s: %v\n%s%s", srcModVers, err, stderr.Bytes(), stdout.Bytes())
	}
	var info struct {
		Dir     string
		Version string
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		log.Fatalf("go mod download -json %s: invalid JSON output: %v\n%s%s", srcModVers, err, stderr.Bytes(), stdout.Bytes())
	}
	return info.Dir, info.Version
}

func currentPackagePath() string {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "list", ".")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("go list: %v\n%s%s", err, stderr.Bytes(), stdout.Bytes())
	}
	return strings.TrimSpace(stdout.String())
}
