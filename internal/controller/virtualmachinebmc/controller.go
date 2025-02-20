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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtualmachinev1 "kubevirt.io/kubevirtbmc/api/v1alpha1"
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
	apiGVStr = virtualmachinev1.GroupVersion.String()
)

func (r *VirtualMachineBMCReconciler) constructPodFromVirtualMachineBMC(virtualMachineBMC *virtualmachinev1.VirtualMachineBMC) *corev1.Pod {
	name := fmt.Sprintf("%s-virtbmc", virtualMachineBMC.Name)
	secretRef := fmt.Sprintf("%s/%s", virtualMachineBMC.Spec.AuthSecret.Namespace, virtualMachineBMC.Spec.AuthSecret.Name)

	// Get the current secret's ResourceVersion to track changes
	secretKey := client.ObjectKey{
		Namespace: virtualMachineBMC.Spec.AuthSecret.Namespace,
		Name:      virtualMachineBMC.Spec.AuthSecret.Name,
	}
	var secret corev1.Secret
	secretVersion := ""
	if err := r.Get(context.Background(), secretKey, &secret); err == nil {
		secretVersion = secret.ResourceVersion
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				VirtualMachineBMCNameLabel: virtualMachineBMC.Name,
				VMNameLabel:                virtualMachineBMC.Spec.VirtualMachine.Name,
			},
			Annotations: map[string]string{
				"lastKnownSecretVersion": secretVersion,
			},
			Name:      name,
			Namespace: VirtualMachineBMCNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  virtBMCContainerName,
					Image: fmt.Sprintf("%s:%s", r.AgentImageName, r.AgentImageTag),
					Args: []string{
						"--address",
						"0.0.0.0",
						"--ipmi-port",
						strconv.Itoa(ipmiPort),
						"--redfish-port",
						strconv.Itoa(redfishPort),
						"--secret-ref",
						secretRef,
						virtualMachineBMC.Spec.VirtualMachine.Namespace,
						virtualMachineBMC.Spec.VirtualMachine.Name,
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          ipmiPortName,
							ContainerPort: ipmiPort,
							Protocol:      corev1.ProtocolUDP,
						},
						{
							Name:          redfishPortName,
							ContainerPort: redfishPort,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
			ServiceAccountName: "kubevirtbmc-virtbmc",
		},
	}

	return pod
}

func (r *VirtualMachineBMCReconciler) constructServiceFromVirtualMachineBMC(virtualMachineBMC *virtualmachinev1.VirtualMachineBMC) *corev1.Service {
	name := fmt.Sprintf("%s-virtbmc", virtualMachineBMC.Name)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				VirtualMachineBMCNameLabel: virtualMachineBMC.Name,
				VMNameLabel:                virtualMachineBMC.Spec.VirtualMachine.Name,
			},
			Name:      name,
			Namespace: VirtualMachineBMCNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				VirtualMachineBMCNameLabel: virtualMachineBMC.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       ipmiPortName,
					Protocol:   corev1.ProtocolUDP,
					TargetPort: intstr.FromString(ipmiPortName),
					Port:       IPMISvcPort,
				},
				{
					Name:       redfishPortName,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString(redfishPortName),
					Port:       RedfishSvcPort,
				},
			},
		},
	}

	return svc
}

//+kubebuilder:rbac:groups=virtualmachine.kubevirt.io,resources=virtualmachinebmcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=virtualmachine.kubevirt.io,resources=virtualmachinebmcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=virtualmachine.kubevirt.io,resources=virtualmachinebmcs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

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

	var virtualMachineBMC virtualmachinev1.VirtualMachineBMC
	if err := r.Get(ctx, req.NamespacedName, &virtualMachineBMC); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch VirtualMachineBMC")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the referenced secret exists
	secretKey := client.ObjectKey{
		Namespace: virtualMachineBMC.Spec.AuthSecret.Namespace,
		Name:      virtualMachineBMC.Spec.AuthSecret.Name,
	}
	var secret corev1.Secret
	secretExists := true
	var secretVersion string
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			secretExists = false
			log.Info("Referenced secret not found", "secret", secretKey)
		} else {
			log.Error(err, "Error fetching secret", "secret", secretKey)
			return ctrl.Result{}, err
		}
	} else {
		secretVersion = secret.ResourceVersion
	}

	// Update the VirtualMachineBMC status based on the existence of the secret
	if secretExists {
		virtualMachineBMC.Status.Conditions = []virtualmachinev1.Condition{
			{
				Type:           "SecretReady",
				Status:         "True",
				LastUpdateTime: metav1.Now(),
			},
		}
	} else {
		virtualMachineBMC.Status.Conditions = []virtualmachinev1.Condition{
			{
				Type:           "SecretReady",
				Status:         "False",
				LastUpdateTime: metav1.Now(),
			},
		}
	}

	// Update the VirtualMachineBMC status
	if err := r.Status().Update(ctx, &virtualMachineBMC); err != nil {
		log.Error(err, "Error updating VirtualMachineBMC status", "vmBMC", virtualMachineBMC.Name)
		return ctrl.Result{}, err
	}
	log.Info("Successfully updated VirtualMachineBMC status", "vmBMC", virtualMachineBMC.Name)

	// Check if pod already exists and if it needs to be refreshed due to secret changes
	podName := fmt.Sprintf("%s-virtbmc", virtualMachineBMC.Name)
	existingPod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: VirtualMachineBMCNamespace}, existingPod)

	if err == nil {
		// Pod exists, check if it needs to be refreshed
		shouldRefresh, reason, refreshErr := r.shouldRefreshPod(ctx, existingPod, &virtualMachineBMC, secretVersion)
		if refreshErr != nil {
			return ctrl.Result{}, refreshErr
		}

		if shouldRefresh {
			log.Info("Pod refresh required", "reason", reason)

			if err := r.Delete(ctx, existingPod, client.GracePeriodSeconds(0)); err != nil {
				log.Error(err, "unable to delete Pod for VirtualMachineBMC", "pod", existingPod)
				return ctrl.Result{}, err
			}
			// Return and let the next reconcile create the new pod
			return ctrl.Result{Requeue: true}, nil
		}

		log.V(1).Info("Pod for VirtualMachineBMC exists and is up-to-date", "pod", podName)
	} else if !apierrors.IsNotFound(err) {
		// Error getting pod that isn't "not found"
		log.Error(err, "Error checking for existing pod", "pod", podName)
		return ctrl.Result{}, err
	} else {
		// Pod doesn't exist, create it
		pod := r.constructPodFromVirtualMachineBMC(&virtualMachineBMC)

		// Add annotation for secret version tracking
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations["lastKnownSecretVersion"] = secretVersion

		if err := ctrl.SetControllerReference(&virtualMachineBMC, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		// Create the virtBMC Pod on the cluster
		if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
			log.Error(err, "unable to create Pod for VirtualMachineBMC", "pod", pod)
			return ctrl.Result{}, err
		}
		log.V(1).Info("created Pod for VirtualMachineBMC", "pod", pod)
	}

	// Prepare the virtBMC Service
	svc := r.constructServiceFromVirtualMachineBMC(&virtualMachineBMC)
	if err := ctrl.SetControllerReference(&virtualMachineBMC, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Create the virtBMC Service on the cluster
	if err := r.Create(ctx, svc); err != nil && !apierrors.IsAlreadyExists(err) {
		log.Error(err, "unable to create Service for VirtualMachineBMC", "svc", svc)
		return ctrl.Result{}, err
	}
	log.V(1).Info("created Service for VirtualMachineBMC", "svc", svc)

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
		For(&virtualmachinev1.VirtualMachineBMC{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				secret := obj.(*corev1.Secret)
				var vmBMCList virtualmachinev1.VirtualMachineBMCList
				if err := r.List(ctx, &vmBMCList); err != nil {
					return nil
				}

				var requests []reconcile.Request
				for _, vmBMC := range vmBMCList.Items {
					if vmBMC.Spec.AuthSecret.Name == secret.Name && vmBMC.Spec.AuthSecret.Namespace == secret.Namespace {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      vmBMC.Name,
								Namespace: vmBMC.Namespace,
							},
						})
					}
				}
				return requests
			}),
		).
		Complete(r)
}

// shouldRefreshPod determines if a pod needs to be refreshed based on secret changes
func (r *VirtualMachineBMCReconciler) shouldRefreshPod(ctx context.Context, existingPod *corev1.Pod, vmBMC *virtualmachinev1.VirtualMachineBMC, secretVersion string) (bool, string, error) {
	log := log.FromContext(ctx)

	// Generate the new secret reference string
	newSecretRef := fmt.Sprintf("%s/%s", vmBMC.Spec.AuthSecret.Namespace, vmBMC.Spec.AuthSecret.Name)

	// Get current secret reference from pod
	currentSecretRef := ""
	for _, container := range existingPod.Spec.Containers {
		if container.Name == virtBMCContainerName {
			for i, arg := range container.Args {
				if arg == "--secret-ref" && i+1 < len(container.Args) {
					currentSecretRef = container.Args[i+1]
					break
				}
			}
			break
		}
	}

	// Check if the secret reference itself has changed
	secretRefChanged := currentSecretRef != newSecretRef

	// Check if the secret data has changed by looking at resource version
	secretDataChanged := false
	if !secretRefChanged && secretVersion != "" {
		// Get the resource version annotation we  have set previously
		lastKnownSecretVersion, hasAnnotation := existingPod.Annotations["lastKnownSecretVersion"]
		if !hasAnnotation || lastKnownSecretVersion != secretVersion {
			secretDataChanged = true
			log.Info("Secret data has changed", "oldVersion", lastKnownSecretVersion, "newVersion", secretVersion)
		}
	}

	reason := ""
	if secretRefChanged {
		reason = "Secret reference changed"
	} else if secretDataChanged {
		reason = "Secret data changed"
	}

	return secretRefChanged || secretDataChanged, reason, nil
}
