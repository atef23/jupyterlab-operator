/*
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
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	jupyterv1alpha1 "github.com/atef23/jupyterlab-operator/api/v1alpha1"
)

// JupyterlabReconciler reconciles a Jupyterlab object
type JupyterlabReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=jupyter.example.com,resources=jupyterlabs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=jupyter.example.com,resources=jupyterlabs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

func (r *JupyterlabReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("jupyterlab", req.NamespacedName)

	// Fetch the Jupyterlab instance
	jupyterlab := &jupyterv1alpha1.Jupyterlab{}
	err := r.Get(ctx, req.NamespacedName, jupyterlab)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Jupyterlab resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Jupyterlab")
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one with a service and route
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: jupyterlab.Name, Namespace: jupyterlab.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForJupyterlab(jupyterlab)
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Ensure the deployment size is the same as the spec
	size := jupyterlab.Spec.Size
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if the service already exists, if not create a new one
	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: jupyterlab.Name, Namespace: jupyterlab.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		// Define a new service
		service := r.serviceForJupyterLab(jupyterlab)
		log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			log.Error(err, "Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			return ctrl.Result{}, err
		}
		// Service created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// Check if the route already exists, if not create a new one
	foundRoute := &routev1.Route{}
	err = r.Get(ctx, types.NamespacedName{Name: jupyterlab.Name, Namespace: jupyterlab.Namespace}, foundRoute)
	if err != nil && errors.IsNotFound(err) {
		// Define a new route
		route := newRoute(jupyterlab.Name, jupyterlab.Namespace, fmt.Sprintf("%s-server", jupyterlab.Name), 8888)
		log.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		ctrl.SetControllerReference(jupyterlab, route, r.Scheme)
		if err := r.Create(ctx, route); err != nil {
			log.Error(err, "Failed to create new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
			return ctrl.Result{}, err
		}
		// Route created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Route")
		return ctrl.Result{}, err
	}

	// Update the Jupyterlab status with the pod names
	// List the pods for this jupyterlab's deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(jupyterlab.Namespace),
		client.MatchingLabels(labelsForJupyterlab(jupyterlab.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods", "Jupyterlab.Namespace", jupyterlab.Namespace, "Jupyterlab.Name", jupyterlab.Name)
		return ctrl.Result{}, err
	}
	podNames := getPodNames(podList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, jupyterlab.Status.Nodes) {
		jupyterlab.Status.Nodes = podNames
		err := r.Status().Update(ctx, jupyterlab)
		if err != nil {
			log.Error(err, "Failed to update Jupyterlab status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// deploymentForJupyterlab returns a jupyterlab Deployment object
func (r *JupyterlabReconciler) deploymentForJupyterlab(m *jupyterv1alpha1.Jupyterlab) *appsv1.Deployment {
	ls := labelsForJupyterlab(m.Name)
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
					Containers: []corev1.Container{
						{
							Name:  "jupyterlab",
							Image: "quay.io/aaziz/jupyterlab:latest",
							Ports: []corev1.ContainerPort{
								{
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 8888,
								},
							},
						},
					},
				},
			},
		},
	}
	// Set Jupyterlab instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// serviceForJupyterLab returns a jupyterLab Service object
func (r *JupyterlabReconciler) serviceForJupyterLab(m *jupyterv1alpha1.Jupyterlab) *corev1.Service {
	//selectors := selectorsForService(m.Name)
	selectors := labelsForJupyterlab(m.Name)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
	}

	service.Spec = corev1.ServiceSpec{
		Ports:    jupyterlabPort.asServicePorts(),
		Selector: selectors,
	}

	// Set JupyterLab instance as the owner and controller
	ctrl.SetControllerReference(m, service, r.Scheme)
	return service
}

func selectorsForService(name string) map[string]string {
	return map[string]string{
		"app": name,
	}
}

// labelsForJupyterlab returns the labels for selecting the resources
// belonging to the given jupyterlab CR name.
func labelsForJupyterlab(name string) map[string]string {
	return map[string]string{"app": "jupyterlab", "jupyterlab_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

func (r *JupyterlabReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jupyterv1alpha1.Jupyterlab{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
