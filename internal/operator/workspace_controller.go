package operator

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	logger.Info("reconcile start", "name", req.Name, "namespace", req.Namespace)
	ws := &apiv1alpha1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, ws); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to get workspace")
		}
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
		if apierrors.IsNotFound(err) {
			createDep = true
			dep = &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: depName, Namespace: ws.Namespace}}
			logger.Info("deployment not found, will create", "name", depName)
		} else {
			logger.Error(err, "get deployment failed")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
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

	// Build environment with defaults. Filter any blank entries to avoid invalid specs.
	var env []corev1.EnvVar
	for _, e := range ws.Spec.Env {
		if strings.TrimSpace(e.Name) == "" { // skip invalid
			continue
		}
		env = append(env, e)
	}
	envIndex := map[string]int{}
	for i, e := range env {
		envIndex[e.Name] = i
	}
	// Ensure PORT=8080 if absent.
	if _, ok := envIndex["PORT"]; !ok {
		env = append(env, corev1.EnvVar{Name: "PORT", Value: "8080"})
	}
	imgLower := strings.ToLower(ws.Spec.Image)
	if strings.Contains(imgLower, "codercom/code-server") || strings.Contains(imgLower, "code-server") {
		if _, exists := envIndex["PASSWORD"]; !exists {
			env = append(env, corev1.EnvVar{Name: "PASSWORD", Value: "changeme"})
		}
	}
	// Container command adjustments for minimal images like alpine that would otherwise exit immediately.
	var command []string
	var readiness *corev1.Probe
	var liveness *corev1.Probe
	if strings.Contains(imgLower, "alpine") {
		// Provide a tiny HTTP responder loop using nc (busybox) so probes can succeed.
		command = []string{"/bin/sh", "-c", "while true; do echo -e 'HTTP/1.1 200 OK\r\nContent-Length:2\r\n\r\nok' | nc -l -p 8080 -w 1; done"}
		// Increased probe windows to avoid premature restarts on slower cold starts.
		readiness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstrFromPort(ports[0])}}, InitialDelaySeconds: 10, PeriodSeconds: 15, TimeoutSeconds: 3, FailureThreshold: 6, SuccessThreshold: 1}
		liveness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstrFromPort(ports[0])}}, InitialDelaySeconds: 60, PeriodSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3, SuccessThreshold: 1}
	} else {
		// Generic images (e.g., code-server) often need extra startup time for unpacking/first-run initialization.
		readiness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstrFromPort(ports[0])}}, InitialDelaySeconds: 20, PeriodSeconds: 15, TimeoutSeconds: 5, FailureThreshold: 8, SuccessThreshold: 1}
		liveness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstrFromPort(ports[0])}}, InitialDelaySeconds: 90, PeriodSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3, SuccessThreshold: 1}
	}

	replicas := int32(1)
	dep.Spec.Replicas = &replicas
	dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"guildnet.io/workspace": ws.Name}}
	dep.Spec.Template.ObjectMeta.Labels = map[string]string{"guildnet.io/workspace": ws.Name}
	dep.Spec.Template.Spec.Containers = []corev1.Container{{
		Name:    "workspace",
		Image:   ws.Spec.Image,
		Env:     env,
		Command: command,
		Ports:   ports,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: func() *bool { b := false; return &b }(),
			RunAsNonRoot:             func() *bool { b := true; return &b }(),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
		ReadinessProbe: readiness,
		LivenessProbe:  liveness,
	}}
	// Pod-level security context
	dep.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsUser:      func() *int64 { v := int64(1000); return &v }(),
		RunAsGroup:     func() *int64 { v := int64(1000); return &v }(),
		FSGroup:        func() *int64 { v := int64(1000); return &v }(),
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
	// Allow scheduling on single control-plane node in Talos dev cluster.
	ctrlPlaneTol := corev1.Toleration{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	// Overwrite tolerations with a single required toleration to prevent duplication drift across reconciles.
	dep.Spec.Template.Spec.Tolerations = []corev1.Toleration{ctrlPlaneTol}

	if err := controllerutil.SetControllerReference(ws, dep, r.Scheme); err != nil {
		logger.Error(err, "set controller ref deployment failed")
		return ctrl.Result{}, err
	}
	if createDep {
		logger.Info("creating deployment", "name", depName)
		if err := r.Create(ctx, dep); err != nil {
			logger.Error(err, "create deployment failed")
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
	} else {
		logger.Info("updating deployment", "name", depName)
		if err := r.Update(ctx, dep); err != nil {
			logger.Error(err, "update deployment failed")
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
	}

	// Service
	svc := &corev1.Service{}
	svcKey := types.NamespacedName{Name: svcName, Namespace: ws.Namespace}
	createSvc := false
	if err := r.Get(ctx, svcKey, svc); err != nil {
		if apierrors.IsNotFound(err) {
			createSvc = true
			svc = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: ws.Namespace}}
			logger.Info("service not found, will create", "name", svcName)
		} else {
			logger.Error(err, "get service failed")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
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
		logger.Error(err, "set controller ref service failed")
		return ctrl.Result{}, err
	}
	if createSvc {
		logger.Info("creating service", "name", svcName)
		if err := r.Create(ctx, svc); err != nil {
			logger.Error(err, "create service failed")
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
	} else {
		logger.Info("updating service", "name", svcName)
		if err := r.Update(ctx, svc); err != nil {
			logger.Error(err, "update service failed")
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
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
	logger.Info("reconcile complete", "phase", ws.Status.Phase, "ready", ws.Status.ReadyReplicas, "svc", ws.Status.ServiceDNS)
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
