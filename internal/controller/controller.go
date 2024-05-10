package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/poneding/virt-vnc-controller/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubevirtcorev1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// VirtualMachineReconciler reconciles a kubevirtcorev1.VirtualMachine object
type VirtualMachineReconciler struct {
	log logr.Logger
	client.Client
	Scheme *runtime.Scheme
}

func (r *VirtualMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log = log.FromContext(ctx)

	vm := &kubevirtcorev1.VirtualMachine{}
	if err := r.Get(ctx, req.NamespacedName, vm); err != nil {
		if k8serrors.IsNotFound(err) {
			// VirtualMachine is deleted, reclaim resources
			if err := r.tryReclaimVirtVNC(ctx, vm); err != nil {
				return ctrl.Result{RequeueAfter: time.Second * 10}, err
			}
		}

		r.log.Error(err, "unable to fetch VirtualMachine")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	r.log.Info("Reconciling VirtualMachine", "Namespace", vm.Namespace, "Name", vm.Name)

	if err := r.tryBuildVirtVNC(ctx, vm); err != nil {
		r.log.Error(err, "unable to build virt-vnc")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *VirtualMachineReconciler) tryBuildVirtVNC(ctx context.Context, vm *kubevirtcorev1.VirtualMachine) error {
	virtVNCName := formatVirtVNCName(vm.Name)
	virtVNCLabels := map[string]string{
		"vm/name":       vm.Name,
		"virt-vnc/name": virtVNCName,
	}

	// Check if the virt-vnc serviceaccount already exists, if not, create it
	if err := r.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: virtVNCName}, &corev1.ServiceAccount{}); err != nil && k8serrors.IsNotFound(err) {
		// Create a new virt-vnc serviceaccount
		serviceaccount := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtVNCName,
				Namespace: vm.Namespace,
				Labels:    virtVNCLabels,
			},
		}

		if err := r.Create(ctx, serviceaccount); err != nil {
			return err
		}
		r.log.Info("Created virt-vnc serviceaccount", "Namespace", vm.Namespace, "Name", virtVNCName)
	}

	// Check if the virt-vnc role already exists, if not, create it
	if err := r.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: virtVNCName}, &rbacv1.Role{}); err != nil && k8serrors.IsNotFound(err) {
		// Create a new virt-vnc role
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtVNCName,
				Namespace: vm.Namespace,
				Labels:    virtVNCLabels,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"subresources.kubevirt.io"},
					Resources: []string{"virtualmachineinstances/vnc", "virtualmachineinstances/console"},
					Verbs:     []string{"get"},
					ResourceNames: []string{
						vm.Name,
					},
				},
			},
		}

		if err := r.Create(ctx, role); err != nil {
			return err
		}
		r.log.Info("Created virt-vnc role", "Namespace", vm.Namespace, "Name", virtVNCName)
	}

	// Check if the virt-vnc rolebinding already exists, if not, create it
	if err := r.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: virtVNCName}, &rbacv1.RoleBinding{}); err != nil && k8serrors.IsNotFound(err) {
		// Create a new virt-vnc rolebinding
		rolebinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtVNCName,
				Namespace: vm.Namespace,
				Labels:    virtVNCLabels,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      virtVNCName,
					Namespace: vm.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     virtVNCName,
				APIGroup: rbacv1.GroupName,
			},
		}

		if err := r.Create(ctx, rolebinding); err != nil {
			return err
		}
		r.log.Info("Created virt-vnc rolebinding", "Namespace", vm.Namespace, "Name", virtVNCName)
	}

	// Check if the virt-vnc deployment already exists, if not, create it
	if err := r.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: virtVNCName}, &appsv1.Deployment{}); err != nil && k8serrors.IsNotFound(err) {
		// Create a new virt-vnc deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtVNCName,
				Namespace: vm.Namespace,
				Labels:    virtVNCLabels,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: util.Ptr[int32](1),
				Selector: &metav1.LabelSelector{
					MatchLabels: virtVNCLabels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: virtVNCLabels,
					},
					Spec: corev1.PodSpec{
						ServiceAccountName: virtVNCName,
						Containers: []corev1.Container{
							{
								Name:  "virt-vnc",
								Image: "poneding/virt-vnc",
								Env: []corev1.EnvVar{
									{
										Name:  "VM_NAMESPACE",
										Value: vm.Namespace,
									},
									{
										Name:  "VM_NAME",
										Value: vm.Name,
									},
								},
								Ports: []corev1.ContainerPort{
									{
										Name:          "vnc",
										ContainerPort: 8080,
									},
								},
								LivenessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										HTTPGet: &corev1.HTTPGetAction{
											Path: "/",
											Port: intstr.IntOrString{
												Type:   intstr.Int,
												IntVal: 8080,
											},
											Scheme: corev1.URISchemeHTTP,
										},
									},
									FailureThreshold:    30,
									InitialDelaySeconds: 30,
									PeriodSeconds:       10,
									SuccessThreshold:    1,
									TimeoutSeconds:      5,
								},
							},
						},
						Tolerations: []corev1.Toleration{
							// Add toleration for the node-role.kubernetes.io/master or node-role.kubernetes.io/control-plane taint
							{
								Key:      "node-role.kubernetes.io/master",
								Operator: corev1.TolerationOpEqual,
								Effect:   corev1.TaintEffectNoSchedule,
							},
							{
								Key:      "node-role.kubernetes.io/control-plane",
								Operator: corev1.TolerationOpEqual,
								Effect:   corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
			},
		}

		if err := r.Create(ctx, deployment); err != nil {
			return err
		}
		r.log.Info("Created virt-vnc deployment", "Namespace", vm.Namespace, "Name", virtVNCName)
	}

	// Check if the virt-vnc service already exists, if not, create it
	if err := r.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: virtVNCName}, &corev1.Service{}); err != nil && k8serrors.IsNotFound(err) {
		// Create a new virt-vnc service
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtVNCName,
				Namespace: vm.Namespace,
				Labels:    virtVNCLabels,
			},
			Spec: corev1.ServiceSpec{
				Selector: virtVNCLabels,
				Ports: []corev1.ServicePort{
					{
						Name:     "vnc",
						Protocol: corev1.ProtocolTCP,
						Port:     8080,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 8080,
						},
					},
				},
			},
		}

		if err := r.Create(ctx, service); err != nil {
			return err
		}
		r.log.Info("Created virt-vnc service", "Namespace", vm.Namespace, "Name", virtVNCName)
	}

	return nil
}

func (r *VirtualMachineReconciler) tryReclaimVirtVNC(ctx context.Context, vm *kubevirtcorev1.VirtualMachine) error {
	virtVNCName := formatVirtVNCName(vm.Name)

	// Delete the virt-vnc service
	if err := r.Delete(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtVNCName,
			Namespace: vm.Namespace,
		},
	}); err != nil && !k8serrors.IsNotFound(err) {
		r.log.Error(err, "unable to delete virt-vnc service", "Namespace", vm.Namespace, "Name", virtVNCName)
		return err
	}

	// Delete the virt-vnc deployment
	if err := r.Delete(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtVNCName,
			Namespace: vm.Namespace,
		},
	}); err != nil && !k8serrors.IsNotFound(err) {
		r.log.Error(err, "unable to delete virt-vnc deployment", "Namespace", vm.Namespace, "Name", virtVNCName)
		return err
	}

	// Delete the virt-vnc rolebinding
	if err := r.Delete(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtVNCName,
			Namespace: vm.Namespace,
		},
	}); err != nil && !k8serrors.IsNotFound(err) {
		r.log.Error(err, "unable to delete virt-vnc rolebinding", "Namespace", vm.Namespace, "Name", virtVNCName)
		return err
	}

	// Delete the virt-vnc role
	if err := r.Delete(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtVNCName,
			Namespace: vm.Namespace,
		},
	}); err != nil && !k8serrors.IsNotFound(err) {
		r.log.Error(err, "unable to delete virt-vnc role", "Namespace", vm.Namespace, "Name", virtVNCName)
		return err
	}

	// Delete the virt-vnc serviceaccount
	if err := r.Delete(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtVNCName,
			Namespace: vm.Namespace,
		},
	}); err != nil && !k8serrors.IsNotFound(err) {
		r.log.Error(err, "unable to delete virt-vnc serviceaccount", "Namespace", vm.Namespace, "Name", virtVNCName)
		return err
	}

	return nil
}

func formatVirtVNCName(vm string) string {
	return vm + "-vnc"
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubevirtcorev1.VirtualMachine{}).
		Complete(r)
}
