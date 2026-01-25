/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package virtualmachinebmc

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"

	bmcv1 "kubevirt.io/kubevirtbmc/api/bmc/v1beta1"
)

var _ = Describe("VirtualMachineBMC Controller", func() {
	const (
		testNamespace = "default"
		timeout       = time.Second * 10
		interval      = time.Millisecond * 250
	)

	type testObjects struct {
		vmName     string
		secretName string
		bmcName    string
	}

	vmKey := func(name string) types.NamespacedName {
		return types.NamespacedName{Name: name, Namespace: testNamespace}
	}

	secretKey := func(name string) types.NamespacedName {
		return types.NamespacedName{Name: name, Namespace: testNamespace}
	}

	bmcKey := func(name string) types.NamespacedName {
		return types.NamespacedName{Name: name, Namespace: testNamespace}
	}

	agentKey := func(vmName string) types.NamespacedName {
		return types.NamespacedName{Name: vmName + "-virtbmc", Namespace: testNamespace}
	}

	serviceAccountKey := func(vmName string) types.NamespacedName {
		return types.NamespacedName{Name: fmt.Sprintf("%s-virtbmc", vmName), Namespace: testNamespace}
	}

	roleBindingKey := func(vmName string) types.NamespacedName {
		return types.NamespacedName{Name: fmt.Sprintf("%s-virtbmc-rolebinding", vmName), Namespace: testNamespace}
	}

	newVM := func(name string) *kubevirtv1.VirtualMachine {
		return &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				Running: boolPtr(false),
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{},
					},
				},
			},
		}
	}

	newSecret := func(name string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("password123"),
			},
		}
	}

	newBMC := func(vmName, secretName, bmcName string, replicas *int32) *bmcv1.VirtualMachineBMC {
		spec := bmcv1.VirtualMachineBMCSpec{
			VirtualMachineRef: &corev1.LocalObjectReference{Name: vmName},
			AuthSecretRef:     &corev1.LocalObjectReference{Name: secretName},
			Instance:          replicas,
		}

		return &bmcv1.VirtualMachineBMC{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmcName,
				Namespace: testNamespace,
			},
			Spec: spec,
		}
	}

	createPrerequisites := func(ctx context.Context, vmName, secretName string) {
		Expect(k8sClient.Create(ctx, newVM(vmName))).To(Succeed())
		Expect(k8sClient.Create(ctx, newSecret(secretName))).To(Succeed())
	}

	createFullSetup := func(ctx context.Context, objs testObjects, replicas *int32) {
		createPrerequisites(ctx, objs.vmName, objs.secretName)
		Expect(k8sClient.Create(ctx, newBMC(objs.vmName, objs.secretName, objs.bmcName, replicas))).To(Succeed())
	}

	expectCondition := func(ctx context.Context, key types.NamespacedName, condType string, status metav1.ConditionStatus, reason string) {
		Eventually(func() bool {
			var bmc bmcv1.VirtualMachineBMC
			if err := k8sClient.Get(ctx, key, &bmc); err != nil {
				return false
			}
			for _, cond := range bmc.Status.Conditions {
				if cond.Type == condType && cond.Status == status {
					return reason == "" || cond.Reason == reason
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())
	}

	expectServiceStatusCleared := func(ctx context.Context, key types.NamespacedName) {
		Eventually(func() bool {
			var bmc bmcv1.VirtualMachineBMC
			if err := k8sClient.Get(ctx, key, &bmc); err != nil {
				return false
			}
			if bmc.Status.ClusterIP != "" {
				return false
			}
			for _, cond := range bmc.Status.Conditions {
				if cond.Type == bmcv1.ConditionReady &&
					cond.Status == metav1.ConditionFalse &&
					cond.Reason == "PrerequisitesMissing" {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())
	}

	waitForServiceAccount := func(ctx context.Context, vmName string) *corev1.ServiceAccount {
		sa := &corev1.ServiceAccount{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, serviceAccountKey(vmName), sa) == nil
		}, timeout, interval).Should(BeTrue())
		return sa
	}

	waitForRoleBinding := func(ctx context.Context, vmName string) *rbacv1.RoleBinding {
		rb := &rbacv1.RoleBinding{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, roleBindingKey(vmName), rb) == nil
		}, timeout, interval).Should(BeTrue())
		return rb
	}

	waitForDeployment := func(ctx context.Context, vmName string) *appsv1.Deployment {
		deployment := &appsv1.Deployment{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, agentKey(vmName), deployment) == nil
		}, timeout, interval).Should(BeTrue())
		return deployment
	}

	waitForService := func(ctx context.Context, vmName string) *corev1.Service {
		svc := &corev1.Service{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, agentKey(vmName), svc) == nil
		}, timeout, interval).Should(BeTrue())
		return svc
	}

	waitForDeploymentDeleted := func(ctx context.Context, vmName string) {
		Eventually(func() bool {
			deployment := &appsv1.Deployment{}
			return errors.IsNotFound(k8sClient.Get(ctx, agentKey(vmName), deployment))
		}, timeout, interval).Should(BeTrue())
	}

	waitForServiceDeleted := func(ctx context.Context, vmName string) {
		Eventually(func() bool {
			svc := &corev1.Service{}
			return errors.IsNotFound(k8sClient.Get(ctx, agentKey(vmName), svc))
		}, timeout, interval).Should(BeTrue())
	}

	Context("resource creation", func() {
		It("creates RBAC resources, Deployment, and Service", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "test-vm", secretName: "test-secret", bmcName: "test-vmbmc"}

			createFullSetup(ctx, objs, nil)

			sa := waitForServiceAccount(ctx, objs.vmName)
			Expect(sa.Name).To(Equal(fmt.Sprintf("%s-virtbmc", objs.vmName)))

			rb := waitForRoleBinding(ctx, objs.vmName)
			Expect(rb.RoleRef.Name).To(Equal(clusterRoleName))
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Name).To(Equal(fmt.Sprintf("%s-virtbmc", objs.vmName)))

			deployment := waitForDeployment(ctx, objs.vmName)
			Expect(deployment.Labels).To(HaveKeyWithValue(VirtualMachineBMCNameLabel, objs.bmcName))
			Expect(deployment.Labels).To(HaveKeyWithValue(VMNameLabel, objs.vmName))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue(VirtualMachineBMCNameLabel, objs.bmcName))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue(VMNameLabel, objs.vmName))
			Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal(fmt.Sprintf("%s-virtbmc", objs.vmName)))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(virtBMCContainerName))
			Expect(deployment.Spec.Replicas).ToNot(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))

			svc := waitForService(ctx, objs.vmName)
			Expect(svc.Labels).To(HaveKeyWithValue(VirtualMachineBMCNameLabel, objs.bmcName))
			Expect(svc.Labels).To(HaveKeyWithValue(VMNameLabel, objs.vmName))
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(VirtualMachineBMCNameLabel, objs.bmcName))
			Expect(svc.Spec.Ports).To(HaveLen(2))
		})

		It("defaults Deployment replicas to 1 when spec.instance is not set", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-default-replicas", secretName: "secret-default-replicas", bmcName: "bmc-default-replicas"}

			createFullSetup(ctx, objs, nil)

			deployment := waitForDeployment(ctx, objs.vmName)
			Expect(deployment.Spec.Replicas).ToNot(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
		})
	})

	Context("status conditions", func() {
		It("sets VirtualMachineAvailable and SecretAvailable when prerequisites exist", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-conditions", secretName: "secret-conditions", bmcName: "bmc-conditions"}

			createFullSetup(ctx, objs, nil)

			expectCondition(ctx, bmcKey(objs.bmcName), bmcv1.ConditionVirtualMachineAvailable, metav1.ConditionTrue, "VirtualMachineFound")
			expectCondition(ctx, bmcKey(objs.bmcName), bmcv1.ConditionSecretAvailable, metav1.ConditionTrue, "SecretFound")
		})

		It("sets VirtualMachineAvailable=False when the referenced VM does not exist", func() {
			ctx := context.Background()

			Expect(k8sClient.Create(ctx, &bmcv1.VirtualMachineBMC{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vmbmc-no-vm",
					Namespace: testNamespace,
				},
				Spec: bmcv1.VirtualMachineBMCSpec{
					VirtualMachineRef: &corev1.LocalObjectReference{Name: "non-existent-vm"},
				},
			})).To(Succeed())

			expectCondition(ctx, bmcKey("test-vmbmc-no-vm"), bmcv1.ConditionVirtualMachineAvailable, metav1.ConditionFalse, "VirtualMachineNotFound")
		})
	})

	Context("prerequisite deletion", func() {
		It("deletes the Deployment and Service and updates status when the VM is deleted", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-delete", secretName: "secret-delete", bmcName: "bmc-vm-delete"}

			createFullSetup(ctx, objs, nil)
			waitForDeployment(ctx, objs.vmName)
			waitForService(ctx, objs.vmName)

			vm := &kubevirtv1.VirtualMachine{}
			Expect(k8sClient.Get(ctx, vmKey(objs.vmName), vm)).To(Succeed())
			Expect(k8sClient.Delete(ctx, vm)).To(Succeed())

			expectCondition(ctx, bmcKey(objs.bmcName), bmcv1.ConditionVirtualMachineAvailable, metav1.ConditionFalse, "VirtualMachineNotFound")
			waitForDeploymentDeleted(ctx, objs.vmName)
			waitForServiceDeleted(ctx, objs.vmName)
			expectServiceStatusCleared(ctx, bmcKey(objs.bmcName))
		})

		It("restores the Deployment and Service when the VM is recreated", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-recreate", secretName: "secret-recreate", bmcName: "bmc-vm-recreate"}

			createFullSetup(ctx, objs, nil)
			waitForDeployment(ctx, objs.vmName)

			vm := &kubevirtv1.VirtualMachine{}
			Expect(k8sClient.Get(ctx, vmKey(objs.vmName), vm)).To(Succeed())
			Expect(k8sClient.Delete(ctx, vm)).To(Succeed())
			waitForDeploymentDeleted(ctx, objs.vmName)
			waitForServiceDeleted(ctx, objs.vmName)

			Expect(k8sClient.Create(ctx, newVM(objs.vmName))).To(Succeed())

			expectCondition(ctx, bmcKey(objs.bmcName), bmcv1.ConditionVirtualMachineAvailable, metav1.ConditionTrue, "VirtualMachineFound")
			waitForDeployment(ctx, objs.vmName)
			waitForService(ctx, objs.vmName)
		})

		It("deletes the Deployment and Service and clears readiness when the Secret is deleted", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-secret-delete", secretName: "secret-to-delete", bmcName: "bmc-secret-delete"}

			createFullSetup(ctx, objs, nil)
			waitForDeployment(ctx, objs.vmName)
			waitForService(ctx, objs.vmName)

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, secretKey(objs.secretName), secret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			expectCondition(ctx, bmcKey(objs.bmcName), bmcv1.ConditionSecretAvailable, metav1.ConditionFalse, "SecretNotFound")
			waitForDeploymentDeleted(ctx, objs.vmName)
			waitForServiceDeleted(ctx, objs.vmName)
			expectServiceStatusCleared(ctx, bmcKey(objs.bmcName))
		})

		It("restores the Deployment and Service when the Secret is recreated", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-secret-restore", secretName: "secret-restore", bmcName: "bmc-secret-restore"}

			createFullSetup(ctx, objs, nil)
			waitForDeployment(ctx, objs.vmName)

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, secretKey(objs.secretName), secret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			waitForDeploymentDeleted(ctx, objs.vmName)
			waitForServiceDeleted(ctx, objs.vmName)

			Expect(k8sClient.Create(ctx, newSecret(objs.secretName))).To(Succeed())

			expectCondition(ctx, bmcKey(objs.bmcName), bmcv1.ConditionSecretAvailable, metav1.ConditionTrue, "SecretFound")
			waitForDeployment(ctx, objs.vmName)
			waitForService(ctx, objs.vmName)
		})
	})

	Context("controller reactions", func() {
		It("updates the Deployment rollout annotation when the Secret changes", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-secret-change", secretName: "secret-to-change", bmcName: "bmc-secret-change"}

			createFullSetup(ctx, objs, nil)

			deployment := waitForDeployment(ctx, objs.vmName)
			originalRestartAt := deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, secretKey(objs.secretName), secret)).To(Succeed())
			secret.Data = map[string][]byte{
				"username": []byte("newadmin"),
				"password": []byte("newpassword456"),
			}
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			Eventually(func() bool {
				updated := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, agentKey(objs.vmName), updated); err != nil {
					return false
				}
				restartedAt := updated.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]
				return restartedAt != "" && restartedAt != originalRestartAt
			}, timeout, interval).Should(BeTrue())
		})

		It("uses VirtualMachineBMC spec.instance for Deployment replicas when it is set", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-scale", secretName: "secret-scale", bmcName: "bmc-scale"}
			replicas := int32(2)

			createFullSetup(ctx, objs, &replicas)

			Eventually(func() bool {
				deployment := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, agentKey(objs.vmName), deployment); err != nil {
					return false
				}
				return deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == replicas
			}, timeout, interval).Should(BeTrue())
		})

		It("reverts manual Deployment edits back to the VirtualMachineBMC-defined state", func() {
			ctx := context.Background()
			objs := testObjects{vmName: "testvm-manual-edit", secretName: "secret-manual-edit", bmcName: "bmc-manual-edit"}
			replicas := int32(2)

			createFullSetup(ctx, objs, &replicas)

			deployment := waitForDeployment(ctx, objs.vmName)
			deployment.Spec.Replicas = int32Ptr(5)
			deployment.Spec.Template.Spec.ServiceAccountName = "manually-edited"
			deployment.Spec.Template.Spec.Containers[0].Name = "manually-edited"
			Expect(k8sClient.Update(ctx, deployment)).To(Succeed())

			Eventually(func() bool {
				current := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, agentKey(objs.vmName), current); err != nil {
					return false
				}
				return current.Spec.Replicas != nil &&
					*current.Spec.Replicas == replicas &&
					current.Spec.Template.Spec.ServiceAccountName == fmt.Sprintf("%s-virtbmc", objs.vmName) &&
					current.Spec.Template.Spec.Containers[0].Name == virtBMCContainerName
			}, timeout, interval).Should(BeTrue())
		})
	})
})

func boolPtr(b bool) *bool {
	return &b
}
