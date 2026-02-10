/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package main

import (
	"os"

	"github.com/michael-niemand/Hortator/cmd/hortator/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
