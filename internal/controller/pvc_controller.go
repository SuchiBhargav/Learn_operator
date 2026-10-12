/*
Copyright 2026.

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
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

const backupLabel = "pvc-backup.example.com/backed-up"

// PVCReconciler watches every PersistentVolumeClaim in the cluster.
type PVCReconciler struct {
	client.Client
	BackupDir string
}

// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;update;patch

// Reconcile runs every time a PVC is created, updated, or deleted.
func (r *PVCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pvc corev1.PersistentVolumeClaim
	err := r.Get(ctx, req.NamespacedName, &pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// The PVC was deleted, so remove its saved YAML.
			if err := r.deleteBackup(req.Namespace, req.Name); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("deleted pvc backup", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if pvc.Labels[backupLabel] != "true" {
		if pvc.Labels == nil {
			pvc.Labels = map[string]string{}
		}
		pvc.Labels[backupLabel] = "true"
		if err := r.Update(ctx, &pvc); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("labeled pvc", "namespace", pvc.Namespace, "name", pvc.Name)
	}

	if err := r.saveBackup(&pvc); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("saved pvc yaml", "namespace", pvc.Namespace, "name", pvc.Name, "dir", r.BackupDir)
	return ctrl.Result{}, nil
}

func (r *PVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

func (r *PVCReconciler) backupPath(namespace, name string) string {
	return filepath.Join(r.BackupDir, fmt.Sprintf("%s_%s.yaml", namespace, name))
}

func (r *PVCReconciler) saveBackup(pvc *corev1.PersistentVolumeClaim) error {
	if err := os.MkdirAll(r.BackupDir, 0o755); err != nil {
		return err
	}

	// The object from the cache has no Kind/APIVersion, so set them
	// before writing a YAML file a person can read.
	out := pvc.DeepCopy()
	out.APIVersion = "v1"
	out.Kind = "PersistentVolumeClaim"
	out.ManagedFields = nil

	data, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(r.backupPath(pvc.Namespace, pvc.Name), data, 0o644)
}

func (r *PVCReconciler) deleteBackup(namespace, name string) error {
	err := os.Remove(r.backupPath(namespace, name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
