package main

import "os"

func main() {
	if err := run(os.Args[1:]); err != nil {
		handleError(err)
	}
}
