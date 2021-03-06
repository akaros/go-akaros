#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROS_KFS_DIR="\$ROSROOT/kern/kfs/bin/"
ERRORSTRINGS_FILE="$DIR/zerrorstrings_${GOOS}_${GOARCH}.go"

export uname=Akaros
export CC=$CC_FOR_TARGET
export GORUN=false
$DIR/mkerrors.sh
if [ "$ROSROOT" != "" ]; then
	mv _errors $ROSROOT/kern/kfs/bin/go_errors
fi
rm -rf _errors _const.go  _error.grep  _error.out  _errors.c  _signal.grep _event.grep

if [ ! -f $ERRORSTRINGS_FILE ]; then
(
cat << 'EOF'
// mkerrors.sh
// MACHINE GENERATED BY THE COMMAND ABOVE;

// PER THE INSTRUCTIONS BELOW, COPY IN THE OUTPUT FROM RUNNING THE
// CROSS-COMPILED EXECUTABLE, 'go_errors' IN YOUR TARGET ENVIRONMENT

package syscall

// Go definitions for mappings from signal/error numbers to strings can be
// obtained by running the "go_errors" executable that has been placed in
// $ROSROOT/kern/kfs/bin/
//
// Unfortunately this executable can not be run in a cross compiled
// environment. Instead, you need to manually launch your target OS and run
// this script to optain these mappings.  Once obtained, simply copy and paste
// them in here for use by the rest of the system.

EOF
) > $ERRORSTRINGS_FILE
fi

cat > /dev/stderr << EOF
  Don't forget to launch Akaros and run the go_errors script that's been placed
  in $ROS_KFS_DIR for you.  If \$ROSROOT was not set, set it now, and then
  rerun this command.  See the comments in the zerrorstrings_${GOOS}_${GOARCH}.go
  file for more information.
EOF

