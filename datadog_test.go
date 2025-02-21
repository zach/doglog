package main

import (
	"fmt"
	"os/user"
	"testing"
)

func ExampleExpand() {
	fmt.Println(Expand("line1\\nthen line2"))
	// Output:
	// line1
	// then line2
}

func TestExpandPath(t *testing.T) {
	path1 := expandPath("~/.doglog")

	usr, _ := user.Current()
	dir := usr.HomeDir

	if path1 != dir+"/.doglog" {
		t.Errorf("expandPath(\"~/.doglog\") = %s", path1)
	}
}
