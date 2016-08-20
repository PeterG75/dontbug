// Copyright © 2016 Sidharth Kshatriya
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"path/filepath"
	"path"
	"github.com/spf13/cobra"
	"os"
	"sort"
	"unsafe"
	"strings"
	"bytes"
	"log"
	"github.com/fatih/color"
)

var gCSkeletonFooter string = `

    return ZEND_USER_OPCODE_DISPATCH;
}
`

var gCSkeletonHeader string = `
/*
 * Copyright 2016 Sidharth Kshatriya
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * This file was autogenerated by dontbug
 * IMPORTANT -- DO NOT remove/edit/move comments with //###
 */
#include "php.h"
#include "php_dontbug.h"

int dontbug_break_location(zend_string* filename, int lineno) {
    zend_ulong hash = filename->h;
    char *cfilename = ZSTR_VAL(filename);
`

type myUintArray []uint64
type myMap map[uint64][]string

func (arr myUintArray) Len() int {
	return len(arr)
}

func (arr myUintArray) Less(i, j int) bool {
	return arr[i] < arr[j]
}

func (arr myUintArray) Swap(i, j int) {
	arr[j], arr[i] = arr[i], arr[j]
}

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate [root-directory]",
	Short: "Generate debug_break.c",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			log.Fatal("dontbug: Please provide root directory of PHP source files on the command line")
		}

		if (len(gExtDir) <= 0) {
			color.Set(color.FgYellow)
			log.Println("dontbug: No --ext-dir provided, assuming \"ext/dontbug\"")
			color.Unset()
			gExtDir = "ext/dontbug"
		}
		generateBreakFile(args[0], gExtDir)
	},
}

func generateBreakFile(rootDir, extDir string) {
	rootDirAbsPath := dirAbsPathOrFatalError(rootDir)
	extDirAbsPath := dirAbsPathOrFatalError(extDir)

	// Open the dontbug_break.c file for generation
	breakFileName := extDirAbsPath + "/dontbug_break.c"
	f, err := os.Create(breakFileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	log.Println("dontbug: Generating", breakFileName, " for all PHP code in", rootDirAbsPath)
	// All is good, now go ahead and do some real work
	ar, m := makeMap(rootDirAbsPath)
	fmt.Fprintln(f, gCSkeletonHeader)
	fmt.Fprintln(f, generateBreakFileBody(ar, m))
	fmt.Fprintln(f, gCSkeletonFooter)
	log.Fatal("dontbug: Generation complete")
}

func init() {
	RootCmd.AddCommand(generateCmd)
	generateCmd.Flags().StringVar(&gExtDir, "ext-dir", "", "")
}

func allFiles(directory string, c chan string) {
	filepath.Walk(directory, func(filepath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// @TODO make this more generic. Get extensions from a yaml file??
		if !info.IsDir() && (path.Ext(filepath) == ".php" || path.Ext(filepath) == ".module") {
			c <- filepath
		}

		return nil
	})
	close(c)
}

// Repeat a space n times
func s(n int) string {
	return strings.Repeat(" ", n)
}

func ifThenElse(ifc, ifb, elseifc, elseifb, elseb string, indent int) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%vif (%v) {\n", s(indent), ifc))
	buf.WriteString(fmt.Sprintf("%v", ifb))
	buf.WriteString(fmt.Sprintf("%v} else if (%v) {\n", s(indent), elseifc))
	buf.WriteString(fmt.Sprintf("%v", elseifb))
	buf.WriteString(fmt.Sprintf("%v} else {\n", s(indent)))
	buf.WriteString(fmt.Sprintf("%v", elseb))
	buf.WriteString(fmt.Sprintf("%v}\n", s(indent)))
	return buf.String()
}

func ifThen(ifc, ifb, elseb string, indent int) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%vif (%v) {\n", s(indent), ifc))
	buf.WriteString(fmt.Sprintf("%v", ifb))
	buf.WriteString(fmt.Sprintf("%v} else {\n", s(indent)))
	buf.WriteString(fmt.Sprintf("%v", elseb))
	buf.WriteString(fmt.Sprintf("%v}\n", s(indent)))
	return buf.String()
}

func eq(rhs uint64) string {
	return fmt.Sprint("hash == ", rhs)
}

func lt(rhs uint64) string {
	return fmt.Sprint("hash < ", rhs)
}

// @TODO deal with hash collisions
func foundHash(hash uint64, matchingFiles []string, indent int) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%v// hash == %v\n", s(indent), hash))
	//buf.WriteString(fmt.Sprintf("%v// %v\n", s(indent), matchingFiles[0]))
	// For a text parser
	// buf.WriteString(fmt.Sprintf("//### %v\n", matchingFiles[0]))
	// Just use the first file for now
	buf.WriteString(fmt.Sprintf("%vreturn ZEND_USER_OPCODE_DISPATCH; //### %v\n", s(indent), matchingFiles[0]))
	return buf.String()
}

// "Daniel J. Bernstein, Times 33 with Addition" string hashing algorithm
// Its the string hashing algorithm used by PHP.
// See https://github.com/php/php-src/blob/PHP-7.0.9/Zend/zend_string.h#L291 for the C language implementation
func djbx33a(byteStr string) uint64 {
	var hash uint64 = 5381
	i := 0

	len := len(byteStr)
	for ; len >= 8; len = len - 8 {
		for j := 0; j < 8; j++ {
			hash = ((hash << 5) + hash) + uint64(byteStr[i])
			i++
		}
	}

	for j := len; j >= 1; j-- {
		hash = ((hash << 5) + hash) + uint64(byteStr[i])
		i++
	}

	if (unsafe.Sizeof(uint(0)) == 8) {
		return hash | (1 << 63)
	} else {
		return hash | (1 << 31)
	}
}

func makeMap(rootdir string) (myUintArray, myMap) {
	c := make(chan string, 100)
	go allFiles(rootdir, c)
	m := make(myMap)
	hash_ar := make(myUintArray, 0, 100)
	for fileName := range c {
		hash := djbx33a(fileName)
		_, ok := m[hash]
		if ok {
			// @TODO make more generic in future
			log.Fatal("Hash collision! Currently unimplemented\n")
			m[hash] = append(m[hash], fileName)
		} else {
			m[hash] = []string{fileName}
			hash_ar = append(hash_ar, hash)
		}
	}
	sort.Sort(hash_ar)
	return hash_ar, m
}

func generateBreakFileBody(arr myUintArray, m myMap) string {
	len := len(arr)
	return generateBreakHelper(arr, m, 0, len - 1, 4)
}

func generateBreakHelper(arr myUintArray, m myMap, low, high, indent int) string {
	if high == low {
		return foundHash(arr[low], m[arr[low]], indent)
	} else {
		var mid int = (high + low) / 2
		if mid == low {
			return ifThen(eq(arr[mid]),
				foundHash(arr[mid], m[arr[mid]], indent + 4),
				foundHash(arr[high], m[arr[high]], indent + 4),
				indent)
		} else {
			return ifThenElse(eq(arr[mid]),
				foundHash(arr[mid], m[arr[mid]], indent + 4),
				lt(arr[mid]),
				generateBreakHelper(arr, m, low, mid, indent + 4),
				generateBreakHelper(arr, m, mid + 1, high, indent + 4),
				indent)
		}
	}
}

