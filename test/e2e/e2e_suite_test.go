//go:build e2e

/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting hortator suite\n")
	RunSpecs(t, "e2e suite")
}
