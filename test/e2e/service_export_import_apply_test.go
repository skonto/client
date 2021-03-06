// Copyright 2020 The Knative Authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build e2e
// +build !eventing

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"gotest.tools/assert"

	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/yaml"

	"knative.dev/client/lib/test"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
)

type expectedServiceOption func(*servingv1.Service)
type expectedRevisionOption func(*servingv1.Revision)
type expectedServiceListOption func(*servingv1.ServiceList)
type expectedRevisionListOption func(*servingv1.RevisionList)
type podSpecOption func(*corev1.PodSpec)

func TestServiceExport(t *testing.T) {
	t.Parallel()
	it, err := test.NewKnTest()
	assert.NilError(t, err)
	defer func() {
		assert.NilError(t, it.Teardown())
	}()

	r := test.NewKnRunResultCollector(t, it)
	defer r.DumpIfFailed()

	t.Log("create service with byo revision")
	serviceCreateWithOptions(r, "hello", "--revision-name", "rev1")

	t.Log("export service-revision1 and compare")
	serviceExport(r, "hello", getServiceWithOptions(
		withServiceName("hello"),
		withServiceRevisionName("hello-rev1"),
		withConfigurationAnnotations(),
		withServicePodSpecOption(withContainer()),
	), "-o", "json")

	t.Log("update service - add env variable")
	test.ServiceUpdate(r, "hello", "--env", "a=mouse", "--revision-name", "rev2", "--no-lock-to-digest")
	serviceExport(r, "hello", getServiceWithOptions(
		withServiceName("hello"),
		withServiceRevisionName("hello-rev2"),
		withServicePodSpecOption(
			withContainer(),
			withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
		),
	), "-o", "json")

	t.Log("export service-revision2 with kubernetes-resources")
	serviceExportWithServiceList(r, "hello", getServiceListWithOptions(
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev2"),
			withTrafficSplit([]string{"latest"}, []int{100}, []string{""}),
			withServicePodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
			),
		),
	), "--with-revisions", "--mode", "kubernetes", "-o", "yaml")

	t.Log("export service-revision2 with revisions-only")
	serviceExportWithRevisionList(r, "hello", getServiceWithOptions(
		withServiceName("hello"),
		withServiceRevisionName("hello-rev2"),
		withTrafficSplit([]string{"latest"}, []int{100}, []string{""}),
		withServicePodSpecOption(
			withContainer(),
			withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
		),
	), getRevisionListWithOptions(), "--with-revisions", "--mode", "resources", "-o", "yaml")

	t.Log("update service with tag and split traffic")
	test.ServiceUpdate(r, "hello", "--tag", "hello-rev1=candidate", "--traffic", "candidate=2%,@latest=98%")

	t.Log("export service-revision2 after tagging kubernetes-resources")
	serviceExportWithServiceList(r, "hello", getServiceListWithOptions(
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev1"),
			withServicePodSpecOption(
				withContainer(),
			),
		),
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev2"),
			withTrafficSplit([]string{"latest", "hello-rev1"}, []int{98, 2}, []string{"", "candidate"}),
			withServicePodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
			),
		),
	), "--with-revisions", "--mode", "kubernetes", "-o", "yaml")

	t.Log("export service-revision2 after tagging with revisions-only")
	serviceExportWithRevisionList(r, "hello", getServiceWithOptions(
		withServiceName("hello"),
		withServiceRevisionName("hello-rev2"),
		withTrafficSplit([]string{"latest", "hello-rev1"}, []int{98, 2}, []string{"", "candidate"}),
		withServicePodSpecOption(
			withContainer(),
			withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
		),
	), getRevisionListWithOptions(
		withRevisions(
			withRevisionName("hello-rev1"),
			withRevisionAnnotations(
				map[string]string{
					"client.knative.dev/user-image": "gcr.io/knative-samples/helloworld-go",
					"serving.knative.dev/creator":   "kubernetes-admin",
				}),
			withRevisionLabels(
				map[string]string{
					"serving.knative.dev/configuration":           "hello",
					"serving.knative.dev/configurationGeneration": "1",
					"serving.knative.dev/route":                   "hello",
					"serving.knative.dev/service":                 "hello",
				}),
			withRevisionPodSpecOption(
				withContainer(),
			),
		),
	), "--with-revisions", "--mode", "resources", "-o", "yaml")

	t.Log("update service - untag, add env variable, traffic split and system revision name")
	test.ServiceUpdate(r, "hello", "--untag", "candidate")
	test.ServiceUpdate(r, "hello", "--env", "b=cat", "--revision-name", "hello-rev3", "--traffic", "hello-rev1=30,hello-rev2=30,hello-rev3=40")

	t.Log("export service-revision3 with kubernetes-resources")
	serviceExportWithServiceList(r, "hello", getServiceListWithOptions(
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev1"),
			withServicePodSpecOption(
				withContainer(),
			),
		),
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev2"),
			withServicePodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
			),
		),
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev3"),
			withTrafficSplit([]string{"hello-rev1", "hello-rev2", "hello-rev3"}, []int{30, 30, 40}, []string{"", "", ""}),
			withServicePodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}, {Name: "b", Value: "cat"}}),
			),
		),
	), "--with-revisions", "--mode", "kubernetes", "-o", "yaml")

	t.Log("export service-revision3 with revisions-only")
	serviceExportWithRevisionList(r, "hello", getServiceWithOptions(
		withServiceName("hello"),
		withServiceRevisionName("hello-rev3"),
		withTrafficSplit([]string{"hello-rev1", "hello-rev2", "hello-rev3"}, []int{30, 30, 40}, []string{"", "", ""}),
		withServicePodSpecOption(
			withContainer(),
			withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}, {Name: "b", Value: "cat"}}),
		),
	), getRevisionListWithOptions(
		withRevisions(
			withRevisionName("hello-rev1"),
			withRevisionAnnotations(
				map[string]string{
					"client.knative.dev/user-image": "gcr.io/knative-samples/helloworld-go",
					"serving.knative.dev/creator":   "kubernetes-admin",
				}),
			withRevisionLabels(
				map[string]string{
					"serving.knative.dev/configuration":           "hello",
					"serving.knative.dev/configurationGeneration": "1",
					"serving.knative.dev/route":                   "hello",
					"serving.knative.dev/service":                 "hello",
				}),
			withRevisionPodSpecOption(
				withContainer(),
			),
		),
		withRevisions(
			withRevisionName("hello-rev2"),
			withRevisionAnnotations(
				map[string]string{
					"serving.knative.dev/creator": "kubernetes-admin",
				}),
			withRevisionLabels(
				map[string]string{
					"serving.knative.dev/configuration":           "hello",
					"serving.knative.dev/configurationGeneration": "2",
					"serving.knative.dev/route":                   "hello",
					"serving.knative.dev/service":                 "hello",
				}),
			withRevisionPodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
			),
		),
	), "--with-revisions", "--mode", "resources", "-o", "yaml")

	t.Log("send all traffic to revision 2")
	test.ServiceUpdate(r, "hello", "--traffic", "hello-rev2=100")

	t.Log("export kubernetes-resources - all traffic to revision 2")
	serviceExportWithServiceList(r, "hello", getServiceListWithOptions(
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev2"),
			withServicePodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
			),
		),
		withServices(
			withServiceName("hello"),
			withServiceRevisionName("hello-rev3"),
			withTrafficSplit([]string{"hello-rev2"}, []int{100}, []string{""}),
			withServicePodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}, {Name: "b", Value: "cat"}}),
			),
		),
	), "--with-revisions", "--mode", "kubernetes", "-o", "yaml")

	t.Log("export revisions-only - all traffic to revision 2")
	serviceExportWithRevisionList(r, "hello", getServiceWithOptions(
		withServiceName("hello"),
		withServiceRevisionName("hello-rev3"),
		withTrafficSplit([]string{"hello-rev2"}, []int{100}, []string{""}),
		withServicePodSpecOption(
			withContainer(),
			withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}, {Name: "b", Value: "cat"}}),
		),
	), getRevisionListWithOptions(
		withRevisions(
			withRevisionName("hello-rev2"),
			withRevisionAnnotations(
				map[string]string{
					"serving.knative.dev/creator": "kubernetes-admin",
				}),
			withRevisionLabels(
				map[string]string{
					"serving.knative.dev/configuration":           "hello",
					"serving.knative.dev/configurationGeneration": "2",
					"serving.knative.dev/route":                   "hello",
					"serving.knative.dev/service":                 "hello",
				}),
			withRevisionPodSpecOption(
				withContainer(),
				withEnv([]corev1.EnvVar{{Name: "a", Value: "mouse"}}),
			),
		),
	), "--with-revisions", "--mode", "resources", "-o", "yaml")
}

// Private methods

func serviceExport(r *test.KnRunResultCollector, serviceName string, expService servingv1.Service, options ...string) {
	command := []string{"service", "export", serviceName}
	command = append(command, options...)
	out := r.KnTest().Kn().Run(command...)
	validateExportedService(r.T(), r.KnTest(), out.Stdout, expService)
	r.AssertNoError(out)
}

func serviceExportWithServiceList(r *test.KnRunResultCollector, serviceName string, expServiceList servingv1.ServiceList, options ...string) {
	command := []string{"service", "export", serviceName}
	command = append(command, options...)
	out := r.KnTest().Kn().Run(command...)
	validateExportedServiceList(r.T(), r.KnTest(), out.Stdout, expServiceList)
	r.AssertNoError(out)
}

func serviceExportWithRevisionList(r *test.KnRunResultCollector, serviceName string, expService servingv1.Service, expRevisionList servingv1.RevisionList, options ...string) {
	command := []string{"service", "export", serviceName}
	command = append(command, options...)
	out := r.KnTest().Kn().Run(command...)
	validateExportedServiceandRevisionList(r.T(), r.KnTest(), out.Stdout, expService, expRevisionList)
	r.AssertNoError(out)
}

// Private functions

func validateExportedService(t *testing.T, it *test.KnTest, out string, expService servingv1.Service) {
	actSvc := servingv1.Service{}
	err := json.Unmarshal([]byte(out), &actSvc)
	assert.NilError(t, err)
	assert.DeepEqual(t, &expService, &actSvc)
}

func validateExportedServiceList(t *testing.T, it *test.KnTest, out string, expServiceList servingv1.ServiceList) {
	actSvcList := servingv1.ServiceList{}
	err := yaml.Unmarshal([]byte(out), &actSvcList)
	assert.NilError(t, err)
	assert.DeepEqual(t, &expServiceList, &actSvcList)
}

func validateExportedServiceandRevisionList(t *testing.T, it *test.KnTest, out string, expService servingv1.Service, expRevisionList servingv1.RevisionList) {
	outArray := strings.Split(out, "apiVersion: v1")

	actSvc := servingv1.Service{}
	err := yaml.Unmarshal([]byte(outArray[0]), &actSvc)
	assert.NilError(t, err)
	assert.DeepEqual(t, &expService, &actSvc)

	if len(outArray) > 1 {
		revListBuilder := strings.Builder{}
		revListBuilder.WriteString("apiVersion: v1")
		revListBuilder.WriteString("\n")
		revListBuilder.WriteString(outArray[1])
		actRevList := servingv1.RevisionList{}
		err := yaml.Unmarshal([]byte(revListBuilder.String()), &actRevList)
		assert.NilError(t, err)
		assert.DeepEqual(t, &actRevList, &actRevList)
	} else if len(expRevisionList.Items) > 0 {
		t.Errorf("expecting a revision list and got no list")
	}
}

func getServiceListWithOptions(options ...expectedServiceListOption) servingv1.ServiceList {
	list := servingv1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "List",
		},
	}

	for _, fn := range options {
		fn(&list)
	}
	return list
}

func withServices(options ...expectedServiceOption) expectedServiceListOption {
	return func(list *servingv1.ServiceList) {
		list.Items = append(list.Items, getServiceWithOptions(options...))
	}
}

func getRevisionListWithOptions(options ...expectedRevisionListOption) servingv1.RevisionList {
	list := servingv1.RevisionList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "List",
		},
	}

	for _, fn := range options {
		fn(&list)
	}

	return list
}

func withRevisions(options ...expectedRevisionOption) expectedRevisionListOption {
	return func(list *servingv1.RevisionList) {
		list.Items = append(list.Items, getRevisionWithOptions(options...))
	}
}

func getServiceWithOptions(options ...expectedServiceOption) servingv1.Service {
	svc := servingv1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "serving.knative.dev/v1",
		},
	}

	for _, fn := range options {
		fn(&svc)
	}
	svc.Spec.Template.Spec.ContainerConcurrency = ptr.Int64(int64(0))
	svc.Spec.Template.Spec.TimeoutSeconds = ptr.Int64(int64(300))

	return svc
}
func withServiceName(name string) expectedServiceOption {
	return func(svc *servingv1.Service) {
		svc.ObjectMeta.Name = name
	}
}
func withConfigurationLabels(labels map[string]string) expectedServiceOption {
	return func(svc *servingv1.Service) {
		svc.Spec.Template.ObjectMeta.Labels = labels
	}
}
func withConfigurationAnnotations() expectedServiceOption {
	return func(svc *servingv1.Service) {
		svc.Spec.Template.ObjectMeta.Annotations = map[string]string{
			"client.knative.dev/user-image": "gcr.io/knative-samples/helloworld-go",
		}
	}
}
func withServiceRevisionName(name string) expectedServiceOption {
	return func(svc *servingv1.Service) {
		svc.Spec.Template.ObjectMeta.Name = name
	}
}
func withTrafficSplit(revisions []string, percentages []int, tags []string) expectedServiceOption {
	return func(svc *servingv1.Service) {
		var trafficTargets []servingv1.TrafficTarget
		for i, rev := range revisions {
			trafficTargets = append(trafficTargets, servingv1.TrafficTarget{
				Percent: ptr.Int64(int64(percentages[i])),
			})
			if tags[i] != "" {
				trafficTargets[i].Tag = tags[i]
			}
			if rev == "latest" {
				trafficTargets[i].LatestRevision = ptr.Bool(true)
			} else {
				trafficTargets[i].RevisionName = rev
				trafficTargets[i].LatestRevision = ptr.Bool(false)
			}
		}
		svc.Spec.RouteSpec = servingv1.RouteSpec{
			Traffic: trafficTargets,
		}
	}
}
func withServicePodSpecOption(options ...podSpecOption) expectedServiceOption {
	return func(svc *servingv1.Service) {
		svc.Spec.Template.Spec.PodSpec = getPodSpecWithOptions(options...)
	}
}
func getRevisionWithOptions(options ...expectedRevisionOption) servingv1.Revision {
	rev := servingv1.Revision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Revision",
			APIVersion: "serving.knative.dev/v1",
		},
	}
	for _, fn := range options {
		fn(&rev)
	}
	rev.Spec.ContainerConcurrency = ptr.Int64(int64(0))
	rev.Spec.TimeoutSeconds = ptr.Int64(int64(300))
	return rev
}
func withRevisionName(name string) expectedRevisionOption {
	return func(rev *servingv1.Revision) {
		rev.ObjectMeta.Name = name
	}
}
func withRevisionLabels(labels map[string]string) expectedRevisionOption {
	return func(rev *servingv1.Revision) {
		rev.ObjectMeta.Labels = labels
	}
}
func withRevisionAnnotations(Annotations map[string]string) expectedRevisionOption {
	return func(rev *servingv1.Revision) {
		rev.ObjectMeta.Annotations = Annotations
	}
}
func withRevisionPodSpecOption(options ...podSpecOption) expectedRevisionOption {
	return func(rev *servingv1.Revision) {
		rev.Spec.PodSpec = getPodSpecWithOptions(options...)
	}
}

func getPodSpecWithOptions(options ...podSpecOption) corev1.PodSpec {
	spec := corev1.PodSpec{}
	for _, fn := range options {
		fn(&spec)
	}
	return spec
}

func withEnv(env []corev1.EnvVar) podSpecOption {
	return func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = env
	}
}

func withContainer() podSpecOption {
	return func(spec *corev1.PodSpec) {
		spec.Containers = []corev1.Container{
			{
				Name:      "user-container",
				Image:     test.KnDefaultTestImage,
				Resources: corev1.ResourceRequirements{},
				ReadinessProbe: &corev1.Probe{
					SuccessThreshold: int32(1),
					Handler: corev1.Handler{
						TCPSocket: &corev1.TCPSocketAction{
							Port: intstr.FromInt(0),
						},
					},
				},
			},
		}
	}
}
