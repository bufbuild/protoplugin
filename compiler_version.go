// Copyright 2024 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protoplugin

import "fmt"

// CompilerVersion is a the version of a compiler provided on a Request.
type CompilerVersion struct {
	// Major is the major version of the compiler.
	//
	// If provided on a Request, will always be >=0.
	Major int
	// Minor is the minor version of the compiler.
	//
	// If provided on a Request, will always be >=0.
	Minor int
	// Patch is the patch version of the compiler.
	//
	// If provided on a Request, will always be >=0.
	Patch int
	// Suffix is the suffix for non-mainline releases.
	//
	// Will be empty for mainline releases.
	Suffix string
}

// String prints the string representation of the CompilerVersion.
//
// If the CompilerVersion is nil, this returns empty.
// If the Patch version is 0 and the Major version is <=3, this returns "Major.Minor[-Suffix]".
// If the Patch version is not 0 or the Major version is >3, this returns "Major.Minor.Patch[-Suffix]".
func (c *CompilerVersion) String() string {
	if c == nil {
		return ""
	}
	var value string
	if c.Major <= 3 || c.Patch != 0 {
		value = fmt.Sprintf("%d.%d.%d", c.Major, c.Minor, c.Patch)
	} else {
		value = fmt.Sprintf("%d.%d", c.Major, c.Minor)
	}
	if c.Suffix != "" {
		return value + "-" + c.Suffix
	}
	return value
}
