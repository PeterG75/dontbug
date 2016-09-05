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

package engine

import (
	"fmt"
	"log"
)

// rr replay sessions are read-only so property_set will always fail
func handlePropertySet(es *engineState, dCmd dbgpCmd) string {
	return fmt.Sprintf(gPropertySetXmlResponseFormat, dCmd.Sequence)
}

// @TODO The stdout/stdin/stderr commands always returns attribute success = "0" until this is implemented
func handleStdFd(es *engineState, dCmd dbgpCmd, fdName string) string {
	return fmt.Sprintf(gStdFdXmlResponseFormat, dCmd.Sequence, fdName)
}

func handleStop(es *engineState, dCmd dbgpCmd) string {
	es.status = statusStopped
	return fmt.Sprintf(gStatusXmlResponseFormat, dCmd.Sequence, es.status, es.reason)
}

func handleInDiversionSessionStandard(es *engineState, dCmd dbgpCmd) string {
	return diversionSessionCmd(es, dCmd.FullCommand)
}

func diversionSessionCmd(es *engineState, command string) string {
	result := xSlashSgdb(es.gdbSession, fmt.Sprintf("dontbug_xdebug_cmd(\"%v\")", command))
	return result
}

// @TODO do we need to do the save/restore of breakpoints?
func handleInDiversionSessionWithNoGdbBpts(es *engineState, dCmd dbgpCmd) string {
	bpList := getEnabledPhpBreakpoints(es)
	disableAllGdbBreakpoints(es)
	result := diversionSessionCmd(es, dCmd.FullCommand)
	enableGdbBreakpoints(es, bpList)
	return result
}

func handleRun(es *engineState, dCmd dbgpCmd, reverse bool) string {
	// Don't hit a breakpoint on your (own) line
	if reverse {
		bpList := getEnabledPhpBreakpoints(es)
		disableGdbBreakpoints(es, bpList)
		// Kind of a step_into backwards
		gotoMasterBpLocation(es, true)
		enableGdbBreakpoints(es, bpList)
	}

	// Resume execution, either forwards or backwards
	_, userBreakPointHit := continueExecution(es, reverse)

	if userBreakPointHit {
		bpList := getEnabledPhpBreakpoints(es)
		disableGdbBreakpoints(es, bpList)
		if !reverse {
			gotoMasterBpLocation(es, false)
		} else {
			// After you hit the php breakpoint, step over backwards.
			currentPhpStackLevel := xSlashDgdb(es.gdbSession, "level")
			id := setPhpStackLevelBreakpointInGdb(es, currentPhpStackLevel)
			continueExecution(es, true)
			removeGdbBreakpoint(es, id)

			// Note that we move in the forward direction even though we are in the reverse case
			gotoMasterBpLocation(es, false)
		}

		filename := xSlashSgdb(es.gdbSession, "filename")
		phpLineno := xSlashDgdb(es.gdbSession, "lineno")

		enableGdbBreakpoints(es, bpList)

		return fmt.Sprintf(gRunOrStepBreakXmlResponseFormat, "run", dCmd.Sequence, filename, phpLineno)
	}

	log.Fatal("Unimplemented program end handling")
	return ""
}

func handleStatus(es *engineState, dCmd dbgpCmd) string {
	return fmt.Sprintf(gStatusXmlResponseFormat, dCmd.Sequence, es.status, es.reason)
}