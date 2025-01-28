#!/bin/sh

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

exitWithError() {
	echo "exitWithError"
	exit 1
}

exitWithSuccess() {
	echo "exitWithSuccess"
	exit 0
}

runForever() {
	echo "runForever"
	while true; do
		sleep 1
	done
}

sleepThenError() {
	echo "sleepThenError"
	sleep .1
	exit 1
}

COUNT=0
exitOnSecondCall() {
	: $((COUNT+=1))
	echo "Function call no ${COUNT}"
	if [ $COUNT -ge 2 ]; then
		exit 0
	fi
}

trapTwice() {
	trap exitOnSecondCall USR1
	echo "Trap registered"
	runForever
}

ARGS="$*"
RUN=

while [ ! -z "$1" ]; do
	case "$1" in
		--argsFile)
			shift
			echo "${ARGS}" >> $1
			;;
		--exitWithError)   RUN=exitWithError;;
		--exitWithSuccess) RUN=exitWithSuccess;;
		--runForever)      RUN=runForever;;
		--sleepThenError)  RUN=sleepThenError;;
		--trapTwice)       RUN=trapTwice;;
	esac
	shift
done

${RUN}
