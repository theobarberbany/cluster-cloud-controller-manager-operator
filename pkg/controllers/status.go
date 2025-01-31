package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/clusteroperator/v1helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The default set of status change reasons.
const (
	ReasonAsExpected          = "AsExpected"
	ReasonInitializing        = "Initializing"
	ReasonSyncing             = "SyncingResources"
	ReasonSyncFailed          = "SyncingFailed"
	ReasonPlatformTechPreview = "PlatformTechPreview"
)

const (
	clusterOperatorName        = "cloud-controller-manager"
	operatorVersionKey         = "operator"
	defaultManagementNamespace = "openshift-cloud-controller-manager-operator"
)

const (
	releaseVersionEnvVariableName = "RELEASE_VERSION"
	unknownVersionValue           = "unknown"
)

type ClusterOperatorStatusClient struct {
	client.Client
	Recorder         record.EventRecorder
	Clock            clock.PassiveClock
	ManagedNamespace string
	ReleaseVersion   string
}

// setStatusDegraded sets the Degraded condition to True, with the given reason and
// message, and sets the upgradeable condition.  It does not modify any existing
// Available or Progressing conditions.
func (r *ClusterOperatorStatusClient) setStatusDegraded(ctx context.Context, reconcileErr error, overrides []configv1.ClusterOperatorStatusCondition) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		klog.Errorf("Failed to get or create Cluster Operator: %v", err)
		return err
	}

	desiredVersions := []configv1.OperandVersion{{Name: operatorVersionKey, Version: r.ReleaseVersion}}
	currentVersions := co.Status.Versions

	var message string
	if !reflect.DeepEqual(desiredVersions, currentVersions) {
		message = fmt.Sprintf("Failed when progressing towards %s because %e", printOperandVersions(desiredVersions), reconcileErr)
	} else {
		message = fmt.Sprintf("Failed to resync for %s because %e", printOperandVersions(desiredVersions), reconcileErr)
	}

	conds := []configv1.ClusterOperatorStatusCondition{
		newClusterOperatorStatusCondition(configv1.OperatorDegraded, configv1.ConditionTrue,
			ReasonSyncFailed, message),
		newClusterOperatorStatusCondition(configv1.OperatorUpgradeable, configv1.ConditionFalse, ReasonAsExpected, ""),
	}

	r.Recorder.Eventf(co, corev1.EventTypeWarning, "Status degraded", reconcileErr.Error())
	klog.V(2).Infof("Syncing status: degraded: %s", message)
	return r.syncStatus(ctx, co, conds, overrides)
}

// setStatusProgressing sets the Progressing condition to True, with the given
// reason and message, and sets the upgradeable condition to True.  It does not
// modify any existing Available or Degraded conditions.
func (r *ClusterOperatorStatusClient) setStatusProgressing(ctx context.Context, overrides []configv1.ClusterOperatorStatusCondition) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		klog.Errorf("Failed to get or create Cluster Operator: %v", err)
		return err
	}

	desiredVersions := []configv1.OperandVersion{{Name: operatorVersionKey, Version: r.ReleaseVersion}}
	currentVersions := co.Status.Versions

	var message, reason string
	if !reflect.DeepEqual(desiredVersions, currentVersions) {
		message = fmt.Sprintf("Progressing towards %s", printOperandVersions(desiredVersions))
		klog.V(2).Infof("Syncing status: %s", message)
		r.Recorder.Eventf(co, corev1.EventTypeNormal, "Status upgrade", message)
		reason = ReasonSyncing
	} else {
		klog.V(2).Info("Syncing status: re-syncing")
		reason = ReasonAsExpected
	}

	conds := []configv1.ClusterOperatorStatusCondition{
		newClusterOperatorStatusCondition(configv1.OperatorProgressing, configv1.ConditionTrue, reason, message),
		newClusterOperatorStatusCondition(configv1.OperatorUpgradeable, configv1.ConditionTrue, ReasonAsExpected, ""),
	}

	return r.syncStatus(ctx, co, conds, overrides)
}

// setStatusAvailable sets the Available condition to True, with the given reason
// and message, and sets both the Progressing and Degraded conditions to False.
func (r *ClusterOperatorStatusClient) setStatusAvailable(ctx context.Context, overrides []configv1.ClusterOperatorStatusCondition) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		return err
	}

	conds := []configv1.ClusterOperatorStatusCondition{
		newClusterOperatorStatusCondition(configv1.OperatorAvailable, configv1.ConditionTrue, ReasonAsExpected,
			fmt.Sprintf("Cluster Cloud Controller Manager Operator is available at %s", r.ReleaseVersion)),
		newClusterOperatorStatusCondition(configv1.OperatorProgressing, configv1.ConditionFalse, ReasonAsExpected, ""),
		newClusterOperatorStatusCondition(configv1.OperatorDegraded, configv1.ConditionFalse, ReasonAsExpected, ""),
		newClusterOperatorStatusCondition(configv1.OperatorUpgradeable, configv1.ConditionTrue, ReasonAsExpected, ""),
	}

	co.Status.Versions = []configv1.OperandVersion{{Name: operatorVersionKey, Version: r.ReleaseVersion}}
	klog.V(2).Info("Syncing status: available")
	return r.syncStatus(ctx, co, conds, overrides)
}

// clearCloudControllerOwnerCondition clears the CloudControllerOwner condition. This condition
// is not used for OpenShift version 4.16 and later as all cloud controllers are external by
// default, and cannot be rolled back to in-tree.
func (r *CloudOperatorReconciler) clearCloudControllerOwnerCondition(ctx context.Context) error {
	co, err := r.getOrCreateClusterOperator(ctx)
	if err != nil {
		return err
	}

	if co.Status.Conditions == nil {
		// no condtions, nothing to do.
		return nil
	}

	if v1helpers.FindStatusCondition(co.Status.Conditions, cloudControllerOwnershipCondition) == nil {
		// condition is not present, nothing to do.
		return nil
	}

	// if we get here, that means the condition exists and we want to remove it
	v1helpers.RemoveStatusCondition(&co.Status.Conditions, cloudControllerOwnershipCondition)
	co.Status.Versions = []configv1.OperandVersion{{Name: operatorVersionKey, Version: r.ReleaseVersion}}
	klog.V(2).Info("Removing CloudControllerOwner condition")
	return r.syncStatus(ctx, co, nil, nil)
}

func printOperandVersions(versions []configv1.OperandVersion) string {
	versionsOutput := []string{}
	for _, operand := range versions {
		versionsOutput = append(versionsOutput, fmt.Sprintf("%s: %s", operand.Name, operand.Version))
	}
	return strings.Join(versionsOutput, ", ")
}

func newClusterOperatorStatusCondition(conditionType configv1.ClusterStatusConditionType,
	conditionStatus configv1.ConditionStatus, reason string,
	message string) configv1.ClusterOperatorStatusCondition {
	return configv1.ClusterOperatorStatusCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

func (r *ClusterOperatorStatusClient) getOrCreateClusterOperator(ctx context.Context) (*configv1.ClusterOperator, error) {
	co := &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterOperatorName,
		},
		Status: configv1.ClusterOperatorStatus{},
	}
	err := r.Get(ctx, client.ObjectKey{Name: clusterOperatorName}, co)
	if errors.IsNotFound(err) {
		klog.Infof("ClusterOperator does not exist, creating a new one.")

		err = r.Create(ctx, co)
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster operator: %v", err)
		}
		return co, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get clusterOperator %q: %v", clusterOperatorName, err)
	}
	return co, nil
}

func (r *ClusterOperatorStatusClient) relatedObjects() []configv1.ObjectReference {
	// TBD: Add an actual set of object references from getResources method
	return []configv1.ObjectReference{
		{Resource: "namespaces", Name: defaultManagementNamespace},
		{Group: configv1.GroupName, Resource: "clusteroperators", Name: clusterOperatorName},
		{Resource: "namespaces", Name: r.ManagedNamespace},
	}
}

// syncStatus applies the new condition to the ClusterOperator object.
func (r *ClusterOperatorStatusClient) syncStatus(ctx context.Context, co *configv1.ClusterOperator, conds, overrides []configv1.ClusterOperatorStatusCondition) error {
	for _, c := range conds {
		v1helpers.SetStatusCondition(&co.Status.Conditions, c, r.Clock)
	}

	// These overrides came from the operator controller and override anything set by the setAvaialble, setProgressing, or setDegraded methods.
	for _, c := range overrides {
		v1helpers.SetStatusCondition(&co.Status.Conditions, c, r.Clock)
	}

	if !equality.Semantic.DeepEqual(co.Status.RelatedObjects, r.relatedObjects()) {
		co.Status.RelatedObjects = r.relatedObjects()
	}

	return r.Status().Update(ctx, co)
}

// GetReleaseVersion gets the release version string from the env
func GetReleaseVersion() string {
	releaseVersion := os.Getenv(releaseVersionEnvVariableName)
	if len(releaseVersion) == 0 {
		releaseVersion = unknownVersionValue
		klog.Infof("%s environment variable is missing, defaulting to %q", releaseVersionEnvVariableName, unknownVersionValue)
	}
	return releaseVersion
}
