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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
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

	// Desired Deployment + Service names.
	depName := ws.Name
	svcName := ws.Name

	// Build desired container ports from spec (with defaults)
	var ports []corev1.ContainerPort
	for _, p := range ws.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{ContainerPort: p.ContainerPort, Name: p.Name, Protocol: p.Protocol})
	}
	if len(ports) == 0 {
		ports = []corev1.ContainerPort{{ContainerPort: 8080, Name: "http"}}
	}

	// Build env (filter blanks, add defaults)
	var env []corev1.EnvVar
	for _, e := range ws.Spec.Env {
		if strings.TrimSpace(e.Name) == "" {
			continue
		}
		env = append(env, e)
	}
	envIndex := map[string]int{}
	for i, e := range env {
		envIndex[e.Name] = i
	}
	if _, ok := envIndex["PORT"]; !ok {
		env = append(env, corev1.EnvVar{Name: "PORT", Value: "8080"})
	}
	imgLower := strings.ToLower(ws.Spec.Image)
	if strings.Contains(imgLower, "codercom/code-server") || strings.Contains(imgLower, "code-server") {
		if _, exists := envIndex["PASSWORD"]; !exists {
			env = append(env, corev1.EnvVar{Name: "PASSWORD", Value: "changeme"})
		}
	}

	// Probes and args
	var command []string
	var readiness *corev1.Probe
	var liveness *corev1.Probe
	var args []string
	probePort := intstr.FromInt(int(ports[0].ContainerPort))
	if strings.Contains(imgLower, "alpine") {
		command = []string{"/bin/sh", "-c", "while true; do echo -e 'HTTP/1.1 200 OK\\r\\nContent-Length:2\\r\\n\\r\\nok' | nc -l -p 8080 -w 1; done"}
		readiness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: probePort}}, InitialDelaySeconds: 10, PeriodSeconds: 5, TimeoutSeconds: 3, FailureThreshold: 12, SuccessThreshold: 1}
		liveness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: probePort}}, InitialDelaySeconds: 60, PeriodSeconds: 15, TimeoutSeconds: 5, FailureThreshold: 3, SuccessThreshold: 1}
	} else {
		// Try /healthz first while keeping "/" as a fallback
		readiness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: probePort}}, InitialDelaySeconds: 10, PeriodSeconds: 5, TimeoutSeconds: 3, FailureThreshold: 12, SuccessThreshold: 1}
		liveness = &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: probePort}}, InitialDelaySeconds: 60, PeriodSeconds: 15, TimeoutSeconds: 5, FailureThreshold: 4, SuccessThreshold: 1}
	}
	if strings.Contains(imgLower, "codercom/code-server") || strings.Contains(imgLower, "code-server") {
		// code-server must bind on 0.0.0.0:8080 w/ password auth; base path handled by proxy
		args = []string{"--bind-addr", "0.0.0.0:8080", "--auth", "password"}
	}

	// Security context
	var secCtx *corev1.SecurityContext
	defFalse := func() *bool { b := false; return &b }()
	defTrue := func() *bool { b := true; return &b }()
	// Default: strict, no privilege escalation, non-root, drop all caps
	secCtx = &corev1.SecurityContext{
		AllowPrivilegeEscalation: defFalse,
		RunAsNonRoot:             defTrue,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
	// Relax for code-server to allow SUID fixuid to run (no_new_privs must be disabled)
	if strings.Contains(imgLower, "codercom/code-server") || strings.Contains(imgLower, "code-server") {
		secCtx = &corev1.SecurityContext{
			AllowPrivilegeEscalation: defTrue,
			RunAsNonRoot:             defTrue,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		}
	}

	// Reconcile Deployment via CreateOrUpdate (handles conflicts) with retry on conflict
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: depName, Namespace: ws.Namespace}}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Use fresh object each try
		dep.ObjectMeta = metav1.ObjectMeta{Name: depName, Namespace: ws.Namespace}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
			dep.Labels = map[string]string{"guildnet.io/workspace": ws.Name}
			replicas := int32(1)
			dep.Spec.Replicas = &replicas
			dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"guildnet.io/workspace": ws.Name}}
			dep.Spec.Template.ObjectMeta.Labels = map[string]string{"guildnet.io/workspace": ws.Name}
			dep.Spec.Template.Spec.Containers = []corev1.Container{{
				Name:            "workspace",
				Image:           ws.Spec.Image,
				Env:             env,
				Command:         command,
				Args:            args,
				Ports:           ports,
				SecurityContext: secCtx,
				ReadinessProbe:  readiness,
				LivenessProbe:   liveness,
			}}
			dep.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsUser:      func() *int64 { v := int64(1000); return &v }(),
				RunAsGroup:     func() *int64 { v := int64(1000); return &v }(),
				FSGroup:        func() *int64 { v := int64(1000); return &v }(),
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			}
			dep.Spec.Template.Spec.Tolerations = []corev1.Toleration{{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
			return controllerutil.SetControllerReference(ws, dep, r.Scheme)
		})
		return err
	}); err != nil {
		logger.Error(err, "reconcile deployment failed")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Reconcile Service via CreateOrUpdate with retry
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: ws.Namespace}}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// fresh object each try
		svc.ObjectMeta = metav1.ObjectMeta{Name: svcName, Namespace: ws.Namespace}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
			svc.Labels = map[string]string{"guildnet.io/workspace": ws.Name}
			svc.Spec.Selector = map[string]string{"guildnet.io/workspace": ws.Name}
			var svcPorts []corev1.ServicePort
			for _, cp := range ports {
				svcPorts = append(svcPorts, corev1.ServicePort{Name: cp.Name, Port: cp.ContainerPort, TargetPort: intstrFromPort(cp)})
			}
			svc.Spec.Ports = svcPorts
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			// Include not-ready addresses so proxy can route during warmup
			svc.Spec.PublishNotReadyAddresses = true
			if ws.Spec.Exposure != nil && ws.Spec.Exposure.Type == apiv1alpha1.ExposureLoadBalancer {
				svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			}
			return controllerutil.SetControllerReference(ws, svc, r.Scheme)
		})
		return err
	}); err != nil {
		logger.Error(err, "reconcile service failed")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Update Status with retry on conflict
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh := &apiv1alpha1.Workspace{}
		if gerr := r.Get(ctx, req.NamespacedName, fresh); gerr != nil {
			return gerr
		}
		fresh.Status.ReadyReplicas = dep.Status.ReadyReplicas
		fresh.Status.ServiceDNS = fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace)
		if svc.Spec.ClusterIP != "" {
			fresh.Status.ServiceIP = svc.Spec.ClusterIP
		}
		switch {
		case fresh.DeletionTimestamp != nil:
			fresh.Status.Phase = apiv1alpha1.PhaseTerminating
		case dep.Status.ReadyReplicas > 0:
			fresh.Status.Phase = apiv1alpha1.PhaseRunning
		default:
			fresh.Status.Phase = apiv1alpha1.PhasePending
		}
		fresh.Status.ProxyTarget = fmt.Sprintf("http://%s:%d", fresh.Status.ServiceDNS, ports[0].ContainerPort)
		return r.Status().Update(ctx, fresh)
	}); err != nil {
		logger.Error(err, "status update failed")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	logger.Info("reconcile complete", "phase", ws.Status.Phase, "ready", ws.Status.ReadyReplicas, "svc", fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace))
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
