//go:build e2e

package e2e_test

import (
	"context"
	"flag"
	"fmt"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	nadsFile  string
	namespace string
)

func init() {
	flag.StringVar(&nadsFile, "nads", "yamls/kind-nads.yaml", "path to NADs YAML file")
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "bond-cni e2e")
}

var _ = BeforeSuite(func(ctx context.Context) {
	Expect(nadsFile).To(BeAnExistingFile())

	namespace = fmt.Sprintf("bond-cni-test-%d", GinkgoRandomSeed())

	out, err := exec.CommandContext(ctx, "kubectl", "create", "namespace", namespace).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))

	out, err = exec.CommandContext(ctx, "kubectl", "apply", "-n", namespace, "-f", nadsFile).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))
})

var _ = AfterSuite(func() {
	exec.Command("kubectl", "delete", "namespace", "--ignore-not-found", namespace).Run()
})
