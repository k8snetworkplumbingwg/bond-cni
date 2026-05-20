//go:build e2e

package e2e_test

import (
	"context"
	"os/exec"
	"path"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("bond", func() {
	When("mode1 (active-backup)", func() {
		const yamlFile = "yamls/bond-mode1.yml"

		BeforeEach(func(ctx context.Context) {
			out, err := exec.CommandContext(ctx, "kubectl", "create", "-n", namespace, "-f", yamlFile).CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))

			DeferCleanup(func() {
				exec.Command("kubectl", "delete", "-n", namespace, "--ignore-not-found", "-f", yamlFile).Run()
			})

			out, err = exec.CommandContext(ctx, "kubectl", "wait", "-n", namespace, "--for=condition=ready", "-l", "app=bond-test", "pod", "--timeout=300s").CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))
		})

		It("works", func(ctx context.Context) {
			out, err := exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, "bond-test", "--",
				"cat", "/proc/net/bonding/bond0").CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))
			bondInfo := string(out)
			Expect(bondInfo).To(ContainSubstring("Bonding Mode: fault-tolerance (active-backup)"))
			Expect(bondInfo).To(ContainSubstring("MII Polling Interval (ms): 100"))

			out, err = exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, "bond-test", "--",
				"cat", "/sys/class/net/bond0/mtu").CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))
			Expect(strings.TrimSpace(string(out))).To(Equal("1300"))
		})
	})

	When("mode2 (balance-xor)", func() {
		DescribeTable("with hash",
			func(ctx context.Context, hash string) {
				yamlFile := "yamls/bond-mode2-" + strings.ReplaceAll(hash, "+", "plus") + ".yml"
				podName := strings.TrimSuffix(path.Base(yamlFile), ".yml")

				By("scheduling a pod on a node with two macvlan interfaces bonded in balance-xor mode")
				out, err := exec.CommandContext(ctx, "kubectl", "create", "-n", namespace, "-f", yamlFile).CombinedOutput()
				Expect(err).NotTo(HaveOccurred(), string(out))

				DeferCleanup(func() {
					exec.Command("kubectl", "delete", "-n", namespace, "--ignore-not-found", "-f", yamlFile).Run()
				})

				By("waiting for the pod to become ready")
				out, err = exec.CommandContext(ctx, "kubectl", "wait", "-n", namespace, "--for=condition=ready",
					"-l", "app="+podName, "pod", "--timeout=300s").CombinedOutput()
				Expect(err).NotTo(HaveOccurred(), string(out))

				By("verifying bond mode and xmit hash policy in /proc/net/bonding/bond0")
				out, err = exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--",
					"cat", "/proc/net/bonding/bond0").CombinedOutput()
				Expect(err).NotTo(HaveOccurred(), string(out))
				bondInfo := string(out)
				Expect(bondInfo).To(ContainSubstring("Bonding Mode: load balancing (xor)"))
				Expect(bondInfo).To(ContainSubstring("Transmit Hash Policy: " + hash))
				Expect(bondInfo).To(ContainSubstring("MII Polling Interval (ms): 100"))

				out, err = exec.CommandContext(ctx, "kubectl", "exec", "-n", namespace, podName, "--",
					"cat", "/sys/class/net/bond0/mtu").CombinedOutput()
				Expect(err).NotTo(HaveOccurred(), string(out))
				Expect(strings.TrimSpace(string(out))).To(Equal("1300"))
			},
			Entry("layer2", "layer2"),
			Entry("layer2+3", "layer2+3"),
		)
	})
})
