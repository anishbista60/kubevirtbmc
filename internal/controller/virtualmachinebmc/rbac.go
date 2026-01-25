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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bmcv1 "kubevirt.io/kubevirtbmc/api/bmc/v1beta1"
)

func (r *VirtualMachineBMCReconciler) constructServiceAccount(virtualMachineBMC *bmcv1.VirtualMachineBMC) *corev1.ServiceAccount {
	serviceAccountName := fmt.Sprintf("%s-virtbmc", virtualMachineBMC.Spec.VirtualMachineRef.Name)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: virtualMachineBMC.Namespace,
		},
	}
	return sa
}

func (r *VirtualMachineBMCReconciler) constructRoleBinding(virtualMachineBMC *bmcv1.VirtualMachineBMC) *rbacv1.RoleBinding {
	serviceAccountName := fmt.Sprintf("%s-virtbmc", virtualMachineBMC.Spec.VirtualMachineRef.Name)
	roleBindingName := fmt.Sprintf("%s-virtbmc-rolebinding", virtualMachineBMC.Spec.VirtualMachineRef.Name)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: virtualMachineBMC.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: virtualMachineBMC.Namespace,
			},
		},
	}
	return rb
}

func (r *VirtualMachineBMCReconciler) ensureRBACResources(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC) error {
	log := log.FromContext(ctx)

	sa := r.constructServiceAccount(virtualMachineBMC)

	if err := ctrl.SetControllerReference(virtualMachineBMC, sa, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, sa); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			log.Error(err, "unable to create ServiceAccount", "serviceAccount", sa.Name)
			return err
		}
	} else {
		log.V(1).Info("created ServiceAccount", "serviceAccount", sa.Name)
	}

	rb := r.constructRoleBinding(virtualMachineBMC)

	if err := ctrl.SetControllerReference(virtualMachineBMC, rb, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, rb); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			log.Error(err, "unable to create RoleBinding", "roleBinding", rb.Name)
			return err
		}
	} else {
		log.V(1).Info("created RoleBinding", "roleBinding", rb.Name)
	}

	return nil
}
