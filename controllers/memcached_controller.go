/*
Copyright 2023.

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

package controllers

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/foroozf001/memcached-operator/api/v1alpha1"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cache.fountain.io,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.fountain.io,resources=memcacheds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.fountain.io,resources=memcacheds/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Memcached object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	memcached := &cachev1alpha1.Memcached{}
	err := r.Get(ctx, req.NamespacedName, memcached)
	if err != nil {
		// ignore when the memcached instance doesn't exist
		if errors.IsNotFound(err) {
			log.Info("memcached instance doesn't exist", "memcached.name", req.Name, "memcached.namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		// failed to get memcached instance. requeue
		log.Error(err, "failed to get memcached instance", "memcached.name", req.Name, "memcached.namespace", req.Namespace)
		return ctrl.Result{}, err
	}
	log.Info("memcached instance found", "memcached.name", req.Name, "memcached.namespace", req.Namespace)

	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      memcached.Name,
		Namespace: memcached.Namespace,
	}, found)
	// deployment doesn't exist. create deployment
	if err != nil && errors.IsNotFound(err) {
		dep := r.deploymentForMemcached(memcached)
		log.Info("deployment doesn't exist", "deployment.name", dep.Name, "deployment.namespace", dep.Namespace)
		err = r.Create(ctx, dep)
		// failed creating deployment
		if err != nil {
			log.Error(err, "failed creating deployment", "deployment.name", dep.Name, "deployment.namespace", dep.Namespace)
			return ctrl.Result{}, err
		}
		// successfully created deployment. requeue
		log.Error(err, "successfully created deployment", "deployment.name", dep.Name, "deployment.namespace", dep.Namespace)
		return ctrl.Result{Requeue: true}, nil

		// failed to get deployment
	} else if err != nil {
		log.Error(err, "failed to get deployment", "deployment.name", found.Name, "deployment.namespace", found.Namespace)
		return ctrl.Result{}, err
	}

	// deployment size is not equal to memcached size
	size := memcached.Spec.Size
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		err = r.Update(ctx, found)
		// failed deployment update
		if err != nil {
			log.Error(err, "failed deployment update", "deployment.name", found.Name, "deployment.namespace", found.Namespace)
			return ctrl.Result{}, err
		}
		// successfully updated deployment. requeue
		log.Info("successfully updated deployment", "deployment.name", found.Name, "deployment.namespace", found.Namespace)
		return ctrl.Result{Requeue: true}, nil
	}

	// list pods
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(memcached.Namespace),
		client.MatchingLabels(labelsForMemcached(memcached.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "failed listing pods", "memcached.namespace", memcached.Namespace)
		return ctrl.Result{}, err
	}
	podNames := getPodNames(podList.Items)
	// update Memcached instance status
	if !reflect.DeepEqual(podNames, memcached.Status.Nodes) {
		memcached.Status.Nodes = podNames
		err = r.Status().Update(ctx, memcached)
		if err != nil {
			log.Error(err, "failed memcached status update", "memcached.name", memcached.Name)
			return ctrl.Result{}, err
		}
	}
	log.Info("successfully updated memcached.Status", "memcached.Status.Nodes", memcached.Status.Nodes)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Memcached{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

// deploymentForMemcached returns a memcached Deployment object
func (r *MemcachedReconciler) deploymentForMemcached(m *cachev1alpha1.Memcached) *appsv1.Deployment {
	ls := labelsForMemcached(m.Name)
	replicas := m.Spec.Size

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:   "memcached:1.4.36-alpine",
						Name:    "memcached",
						Command: []string{"memcached", "-m=64", "-o", "modern", "-v"},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 11211,
							Name:          "memcached",
						}},
					}},
				},
			},
		},
	}
	// Set Memcached instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// labelsForMemcached returns the labels belonging to the given memcached CR name.
func labelsForMemcached(name string) map[string]string {
	return map[string]string{
		"app":          "memcached",
		"memcached_cr": name,
	}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}
