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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/util/intstr"

	bmcv1 "kubevirt.io/kubevirtbmc/api/bmc/v1beta1"
)

func (r *VirtualMachineBMCReconciler) createVirtBMCService(virtualMachineBMC *bmcv1.VirtualMachineBMC) *corev1.Service {
	name := fmt.Sprintf("%s-virtbmc", virtualMachineBMC.Spec.VirtualMachineRef.Name)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				VirtualMachineBMCNameLabel: virtualMachineBMC.Name,
				VMNameLabel:                virtualMachineBMC.Spec.VirtualMachineRef.Name,
			},
			Name:      name,
			Namespace: virtualMachineBMC.Namespace,
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

func (r *VirtualMachineBMCReconciler) ensureVirtBMCService(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC) error {
	log := log.FromContext(ctx)

	svc := r.createVirtBMCService(virtualMachineBMC)
	if err := ctrl.SetControllerReference(virtualMachineBMC, svc, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, svc); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		log.Error(err, "unable to create Service for VirtualMachineBMC", "service", svc.Name)
		return err
	}

	log.V(1).Info("created Service for VirtualMachineBMC", "service", svc.Name)
	return nil
}

func (r *VirtualMachineBMCReconciler) deleteVirtBMCService(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC) error {
	log := log.FromContext(ctx)
	service, err := r.getVirtBMCService(ctx, virtualMachineBMC)

	if err != nil {
		return err
	}

	if service == nil {
		return nil
	}

	if err := r.Delete(ctx, service); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "unable to delete virtBMC Service", "service", service.Name)
		return err
	}

	log.Info("deleted virtBMC Service", "service", service.Name)
	return nil
}

func (r *VirtualMachineBMCReconciler) getVirtBMCService(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC) (*corev1.Service, error) {
	log := log.FromContext(ctx)
	serviceName := fmt.Sprintf("%s-virtbmc", virtualMachineBMC.Spec.VirtualMachineRef.Name)

	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: virtualMachineBMC.Namespace,
		Name:      serviceName,
	}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		log.Error(err, "unable to get virtBMC Service", "service", serviceName)
		return nil, err
	}

	return svc, nil
}

func (r *VirtualMachineBMCReconciler) reconcileVirtBMCStatus(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC) error {
	log := log.FromContext(ctx)

	svc, err := r.getVirtBMCService(ctx, virtualMachineBMC)
	if err != nil {
		return err
	}

	if svc == nil {
		return nil
	}

	if svc.Spec.ClusterIP == "" {
		return fmt.Errorf("clusterIP is not ready yet")
	}

	if changed := meta.SetStatusCondition(
		&virtualMachineBMC.Status.Conditions,
		metav1.Condition{
			Type:    bmcv1.ConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  "ServiceReady",
			Message: "ClusterIP assigned to the Service",
		},
	); changed {
		if err := r.Status().Update(ctx, virtualMachineBMC); err != nil {
			return err
		}
	}

	virtualMachineBMC.Status.ClusterIP = svc.Spec.ClusterIP
	if err := r.Status().Update(ctx, virtualMachineBMC); err != nil {
		log.Error(err, "unable to update VirtualMachineBMC status")
		return err
	}

	log.V(1).Info("updated VirtualMachineBMC status for Service", "virtualMachineBMC", virtualMachineBMC.Name)

	return nil
}

func (r *VirtualMachineBMCReconciler) clearVirtBMCServiceStatus(ctx context.Context, virtualMachineBMC *bmcv1.VirtualMachineBMC, reason, message string) error {
	if changed := meta.SetStatusCondition(
		&virtualMachineBMC.Status.Conditions,
		metav1.Condition{
			Type:    bmcv1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  reason,
			Message: message,
		},
	); changed {
		virtualMachineBMC.Status.ClusterIP = ""
		return r.Status().Update(ctx, virtualMachineBMC)
	}

	if virtualMachineBMC.Status.ClusterIP == "" {
		return nil
	}

	virtualMachineBMC.Status.ClusterIP = ""
	return r.Status().Update(ctx, virtualMachineBMC)
}
