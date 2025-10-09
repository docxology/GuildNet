package operator

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiv1alpha1 "github.com/your/module/api/v1alpha1"
)

// WorkspaceReconciler reconciles a Workspace object into a Deployment + Service.
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile implements the reconciliation loop.
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ws := &apiv1alpha1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, ws); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion (no finalizer for prototype) just ensure owned resources disappear with ownerRefs.

	// Desired Deployment + Service names.
	depName := ws.Name
	svcName := ws.Name

	// Build or update Deployment.
	dep := &appsv1.Deployment{}
	depKey := types.NamespacedName{Name: depName, Namespace: ws.Namespace}
	createDep := false
	if err := r.Get(ctx, depKey, dep); err != nil {
		createDep = true
		dep = &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: depName, Namespace: ws.Namespace}}
	}

	// Spec template (simple single container).
	var ports []corev1.ContainerPort
	for _, p := range ws.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{ContainerPort: p.ContainerPort, Name: p.Name, Protocol: p.Protocol})
	}
	if len(ports) == 0 {
		// default port 8080
		ports = []corev1.ContainerPort{{ContainerPort: 8080, Name: "http"}}
	}

	replicas := int32(1)
	dep.Spec.Replicas = &replicas
	dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"guildnet.io/workspace": ws.Name}}
	dep.Spec.Template.ObjectMeta.Labels = map[string]string{"guildnet.io/workspace": ws.Name}
	dep.Spec.Template.Spec.Containers = []corev1.Container{{
		Name:           "workspace",
		Image:          ws.Spec.Image,
		Env:            ws.Spec.Env,
		Ports:          ports,
		ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstrFromPort(ports[0])}}, InitialDelaySeconds: 2, PeriodSeconds: 5},
		LivenessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstrFromPort(ports[0])}}, InitialDelaySeconds: 5, PeriodSeconds: 10},
	}}

	if err := controllerutil.SetControllerReference(ws, dep, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if createDep {
		if err := r.Create(ctx, dep); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.Update(ctx, dep); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Service
	svc := &corev1.Service{}
	svcKey := types.NamespacedName{Name: svcName, Namespace: ws.Namespace}
	createSvc := false
	if err := r.Get(ctx, svcKey, svc); err != nil {
		createSvc = true
		svc = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: ws.Namespace}}
	}
	svc.Spec.Selector = map[string]string{"guildnet.io/workspace": ws.Name}
	var svcPorts []corev1.ServicePort
	for _, cp := range ports {
		svcPorts = append(svcPorts, corev1.ServicePort{Name: cp.Name, Port: cp.ContainerPort, TargetPort: intstrFromPort([]corev1.ContainerPort{cp}[0])})
	}
	svc.Spec.Ports = svcPorts
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	if ws.Spec.Exposure != nil && ws.Spec.Exposure.Type == apiv1alpha1.ExposureLoadBalancer {
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	}
	if err := controllerutil.SetControllerReference(ws, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if createSvc {
		if err := r.Create(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update Status (basic)
	ws.Status.ReadyReplicas = dep.Status.ReadyReplicas
	ws.Status.ServiceDNS = fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace)
	if svc.Spec.ClusterIP != "" {
		ws.Status.ServiceIP = svc.Spec.ClusterIP
	}
	// Determine phase
	switch {
	case ws.DeletionTimestamp != nil:
		ws.Status.Phase = apiv1alpha1.PhaseTerminating
	case dep.Status.ReadyReplicas > 0:
		ws.Status.Phase = apiv1alpha1.PhaseRunning
	default:
		ws.Status.Phase = apiv1alpha1.PhasePending
	}
	// Proxy target uses service DNS + first port
	ws.Status.ProxyTarget = fmt.Sprintf("http://%s:%d", ws.Status.ServiceDNS, ports[0].ContainerPort)
	if err := r.Status().Update(ctx, ws); err != nil {
		logger.Error(err, "status update failed")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func intstrFromPort(p corev1.ContainerPort) intstr.IntOrString {
	return intstr.FromInt(int(p.ContainerPort))
}

// SetupWithManager wires controller to manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.Workspace{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
