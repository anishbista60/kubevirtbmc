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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bmcv1 "kubevirt.io/kubevirtbmc/api/bmc/v1beta1"
)

// VirtualMachineBMCReconciler reconciles a VirtualMachineBMC object
type VirtualMachineBMCReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	AgentImageName string
	AgentImageTag  string
}

var (
	ownerKey = ".metadata.controller"
	apiGVStr = bmcv1.GroupVersion.String()
)

func (r *VirtualMachineBMCReconciler) validateVirtualMachineExists(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC) (bool, error) {
	log := log.FromContext(ctx)

	vmKey := client.ObjectKey{
		Namespace: virtualMachineBMC.Namespace,
		Name:      virtualMachineBMC.Spec.VirtualMachineRef.Name,
	}

	var vm kubevirtv1.VirtualMachine
	if err := r.Get(ctx, vmKey, &vm); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Referenced VirtualMachine not found",
				"vm", virtualMachineBMC.Spec.VirtualMachineRef.Name,
				"namespace", virtualMachineBMC.Namespace)

			if changed := meta.SetStatusCondition(&virtualMachineBMC.Status.Conditions, metav1.Condition{
				Type:   bmcv1.ConditionVirtualMachineAvailable,
				Status: metav1.ConditionFalse,
				Reason: "VirtualMachineNotFound",
				Message: fmt.Sprintf("VirtualMachine %q not found in namespace %q",
					virtualMachineBMC.Spec.VirtualMachineRef.Name,
					virtualMachineBMC.Namespace),
			}); changed {
				return false, r.Status().Update(ctx, virtualMachineBMC)
			}
			return false, nil
		}
		log.Error(err, "error checking VirtualMachine existence")
		return false, err
	}

	if changed := meta.SetStatusCondition(&virtualMachineBMC.Status.Conditions, metav1.Condition{
		Type:    bmcv1.ConditionVirtualMachineAvailable,
		Status:  metav1.ConditionTrue,
		Reason:  "VirtualMachineFound",
		Message: fmt.Sprintf("VirtualMachine %q is available", virtualMachineBMC.Spec.VirtualMachineRef.Name),
	}); changed {
		return false, r.Status().Update(ctx, virtualMachineBMC)
	}
	return true, nil
}

func (r *VirtualMachineBMCReconciler) validateSecretExists(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC) (bool, error) {
	log := log.FromContext(ctx)

	if virtualMachineBMC.Spec.AuthSecretRef == nil {
		log.Info("AuthSecretRef is not set, skipping secret validation")
		return false, nil
	}

	secretKey := client.ObjectKey{
		Namespace: virtualMachineBMC.Namespace,
		Name:      virtualMachineBMC.Spec.AuthSecretRef.Name,
	}

	var secret corev1.Secret
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Referenced Secret not found",
				"secret", virtualMachineBMC.Spec.AuthSecretRef.Name,
				"namespace", virtualMachineBMC.Namespace)

			if changed := meta.SetStatusCondition(&virtualMachineBMC.Status.Conditions, metav1.Condition{
				Type:   bmcv1.ConditionSecretAvailable,
				Status: metav1.ConditionFalse,
				Reason: "SecretNotFound",
				Message: fmt.Sprintf("Secret %q not found in namespace %q",
					virtualMachineBMC.Spec.AuthSecretRef.Name,
					virtualMachineBMC.Namespace),
			}); changed {
				return false, r.Status().Update(ctx, virtualMachineBMC)

			}
			return false, nil
		}
		log.Error(err, "error checking Secret existence")
		return false, err
	}

	if changed := meta.SetStatusCondition(&virtualMachineBMC.Status.Conditions, metav1.Condition{
		Type:    bmcv1.ConditionSecretAvailable,
		Status:  metav1.ConditionTrue,
		Reason:  "SecretFound",
		Message: fmt.Sprintf("Secret %q is available", virtualMachineBMC.Spec.AuthSecretRef.Name),
	}); changed {
		return false, r.Status().Update(ctx, virtualMachineBMC)
	}
	return true, nil
}

//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachines,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=bmc.kubevirt.io,resources=virtualmachinebmcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.kubevirt.io,resources=virtualmachinebmcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.kubevirt.io,resources=virtualmachinebmcs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;services;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VirtualMachineBMC object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *VirtualMachineBMCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var virtualMachineBMC bmcv1.VirtualMachineBMC
	if err := r.Get(ctx, req.NamespacedName, &virtualMachineBMC); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch VirtualMachineBMC")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	vmExists, err := r.validateVirtualMachineExists(ctx, &virtualMachineBMC)
	if err != nil {
		return ctrl.Result{}, err
	}

	secretExists, err := r.validateSecretExists(ctx, &virtualMachineBMC)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !vmExists || !secretExists {
		if err := r.deleteVirtBMCDeployment(ctx, &virtualMachineBMC); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.deleteVirtBMCService(ctx, &virtualMachineBMC); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.clearVirtBMCServiceStatus(ctx, &virtualMachineBMC, "PrerequisitesMissing", "Service is unavailable because VirtualMachineBMC prerequisites are not met"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if virtualMachineBMC.Spec.VirtualMachineRef == nil || !vmExists || virtualMachineBMC.Spec.AuthSecretRef == nil || !secretExists {
		log.Info("Prerequisites not met, skipping reconciling",
			"vmExists", vmExists,
			"secretExists", secretExists,
			"authSecretRefSet", virtualMachineBMC.Spec.AuthSecretRef != nil)
		return ctrl.Result{}, nil
	}

	if err := r.ensureRBACResources(ctx, &virtualMachineBMC); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ensureVirtBMCDeployment(ctx, &virtualMachineBMC); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.scaleVirtBMCDeploymentReplicas(ctx, &virtualMachineBMC); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.ensureVirtBMCService(ctx, &virtualMachineBMC); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileVirtBMCStatus(ctx, &virtualMachineBMC); err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualMachineBMCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, ownerKey, func(rawObj client.Object) []string {
		// grab the pod object, extract the owner...
		pod := rawObj.(*corev1.Pod)
		owner := metav1.GetControllerOf(pod)
		if owner == nil {
			return nil
		}
		// ...make sure it's a VirtualMachineBMC...
		if owner.APIVersion != apiGVStr || owner.Kind != "VirtualMachineBMC" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, ownerKey, func(rawObj client.Object) []string {
		// grab the svc object, extract the owner...
		svc := rawObj.(*corev1.Service)
		owner := metav1.GetControllerOf(svc)
		if owner == nil {
			return nil
		}
		// ...make sure it's a VirtualMachineBMC...
		if owner.APIVersion != apiGVStr || owner.Kind != "VirtualMachineBMC" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bmcv1.VirtualMachineBMC{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(
			&kubevirtv1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(r.findVirtualMachineBMCsForSecretAndVM),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findVirtualMachineBMCsForSecretAndVM),
		).
		Complete(r)
}

func (r *VirtualMachineBMCReconciler) findVirtualMachineBMCsForSecretAndVM(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	var vmBMCList bmcv1.VirtualMachineBMCList
	if err := r.List(ctx, &vmBMCList, client.InNamespace(obj.GetNamespace())); err != nil {
		log.Error(err, "unable to list VirtualMachineBMCs")
		return nil
	}

	var requests []reconcile.Request

	for _, vmBMC := range vmBMCList.Items {
		match := false

		switch o := obj.(type) {
		case *kubevirtv1.VirtualMachine:
			if vmBMC.Spec.VirtualMachineRef != nil && vmBMC.Spec.VirtualMachineRef.Name == o.GetName() {
				match = true
			}

		case *corev1.Secret:
			if vmBMC.Spec.AuthSecretRef != nil && vmBMC.Spec.AuthSecretRef.Name == o.GetName() {
				vmBMCCopy := vmBMC.DeepCopy()
				if err := r.rolloutRestartVirtBMCDeployment(ctx, vmBMCCopy); err != nil {
					log.Error(err, "unable to rollout restart virtBMC Deployment during Secret change", "vmBMC", vmBMC.Name)
				}
				log.Info("Rolled out restart of virtBMC Deployment after Secret change")

				match = true
			}
		}

		if match {
			log.Info("Enqueueing VirtualMachineBMC", "changedObject", obj.GetName(), "vmBMC", vmBMC.Name)

			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      vmBMC.Name,
					Namespace: vmBMC.Namespace,
				},
			})
		}
	}

	return requests
}
