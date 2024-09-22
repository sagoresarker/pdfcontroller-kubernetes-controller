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

package controller

import (
	"context"
	"encoding/base64"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8sstartkubernetescomv1 "k8s.startkubernetes.com/api/v1"
)

// PdfDocumentReconciler reconciles a PdfDocument object
type PdfDocumentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=k8s.startkubernetes.com.my.domain,resources=pdfdocuments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.startkubernetes.com.my.domain,resources=pdfdocuments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8s.startkubernetes.com.my.domain,resources=pdfdocuments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PdfDocument object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *PdfDocumentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pdfDoc k8sstartkubernetescomv1.PdfDocument
	if err := r.Get(ctx, req.NamespacedName, &pdfDoc); err != nil {
		logger.Error(err, "unable to fetch PdfDocument")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("PdfDocument fetched", "DocumentName", pdfDoc.Spec.DocumentName)

	jobSpec, err := r.CreateJob(pdfDoc)
	if err != nil {
		logger.Error(err, "unable to create job spec")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, jobSpec); err != nil {
		logger.Error(err, "unable to create job")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// CreateJob creates a job spec for the PdfDocument with 2 init containers
func (r *PdfDocumentReconciler) CreateJob(pdfDoc k8sstartkubernetescomv1.PdfDocument) (*batchv1.Job, error) {
	image := "knsit/pandoc"
	base64text := base64.StdEncoding.EncodeToString([]byte(pdfDoc.Spec.Text))

	// Create a new job
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdfDoc.Spec.DocumentName,
			Namespace: pdfDoc.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,

					InitContainers: []corev1.Container{
						{
							Name:    "store-to-md",
							Image:   "alpine",
							Command: []string{"/bin/sh"},
							Args:    []string{"-c", fmt.Sprintf("echo %s | base64 -d > /data/text.md", base64text)},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data-volume",
									MountPath: "/data",
								},
							},
						},
						{
							Name:    "convert",
							Image:   image,
							Command: []string{"sh", "-c"},
							Args:    []string{fmt.Sprintf("pandoc -s -o /data/%s.pdf /data/text.md", pdfDoc.Spec.DocumentName)},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data-volume",
									MountPath: "/data",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "main",
							Image:   "alpine",
							Command: []string{"sh", "-c", "sleep 36000"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data-volume",
									MountPath: "/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	// Set PdfDocument instance as the owner and controller
	if err := ctrl.SetControllerReference(&pdfDoc, job, r.Scheme); err != nil {
		return nil, err
	}

	return job, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PdfDocumentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sstartkubernetescomv1.PdfDocument{}).
		Complete(r)
}
