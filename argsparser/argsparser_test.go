package argsparser

import (
	"strconv"
	"testing"
)

// Helper function to assert that two strings are equal
func checkStringsEqual(t *testing.T, expected string, actual string) {
	if expected != actual {
		t.Fatal("Actual response: " + actual + " does not match expected: " + expected)
	}
}

func TestParseArgs(t *testing.T) {
	argsArray, err := ParseArgs("\"arg with quotes\" secondArg thirdArg \"another with quotes\"")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(argsArray) != 4 {
		t.Fatal("Args length = " + strconv.Itoa(len(argsArray)) + ", 4 expected")
	}
	checkStringsEqual(t, "arg with quotes", argsArray[0])
	checkStringsEqual(t, "secondArg", argsArray[1])
	checkStringsEqual(t, "thirdArg", argsArray[2])
	checkStringsEqual(t, "another with quotes", argsArray[3])
}
