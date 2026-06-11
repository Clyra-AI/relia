package main

import (
	"fmt"
	"io"
	"os"
)

func appName() string {
	return "relia"
}

func run(stdout io.Writer) error {
	_, err := fmt.Fprintln(stdout, appName())
	return err
}

func exitCode(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

func main() {
	if code := exitCode(run(os.Stdout)); code != 0 {
		os.Exit(code)
	}
}
