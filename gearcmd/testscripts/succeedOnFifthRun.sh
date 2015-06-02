#!/bin/bash

# This script fails when it is run the first four times and succeeds on the fifth run.
# It keeps track of how many times it has been run using an input file arg.

read NUM_TIMES_RUN < $1
if [ -z "$NUM_TIMES_RUN" ]; then
  NUM_TIMES_RUN="0"
fi

((NUM_TIMES_RUN++))
echo "Script is on run $NUM_TIMES_RUN."
if [ "$NUM_TIMES_RUN" -le "4" ]; then
  echo "Going To Fail"
  echo $NUM_TIMES_RUN > $1
  exit 2
else
  echo "Going To Succeed"
  NUM_TIMES_RUN=0
  echo $NUM_TIMES_RUN > $1
fi
