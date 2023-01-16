package controllers

import (
	"context"
	"fmt"
	"time"

	cachev1alpha1 "github.com/foroozf001/memcached-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MemcachedConfig struct {
	ApiVersion string
	Kind       string
	Name       string
	Namespace  string
	Size       int32
	timeout    time.Duration
	interval   time.Duration
}

var _ = Describe("test memcacheds.cache.fountain.io/v1alphav1", func() {
	var config = MemcachedConfig{
		ApiVersion: "cache.fountain.io/v1alphav1",
		Kind:       "Memcached",
		Name:       "memcached-sample",
		Namespace:  "test",
		Size:       3,
		timeout:    time.Second * 30,
		interval:   time.Second * 1,
	}

	ctx := context.Background()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: config.Namespace,
		},
	}

	typeNamespaceName := types.NamespacedName{Name: config.Name, Namespace: namespace.Name}

	BeforeEach(func() {
		By("creating the namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))
	})

	AfterEach(func() {
		// TODO(user): Attention if you improve this code by adding other context test you MUST
		// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("deleting the namespace to perform the tests")
		err := k8sClient.Delete(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))
	})

	It("should successfully reconcile a custom resource for memcached", func() {
		By("creating the custom resource for the kind memcached")
		// Let's mock our custom resource at the same way that we would
		// apply on the cluster the manifest under config/samples
		memcached := &cachev1alpha1.Memcached{
			TypeMeta: metav1.TypeMeta{
				APIVersion: config.ApiVersion,
				Kind:       config.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.Name,
				Namespace: namespace.Name,
			},
			Spec: cachev1alpha1.MemcachedSpec{
				Size: config.Size,
			},
		}

		err := k8sClient.Create(ctx, memcached)
		Expect(err).To(Not(HaveOccurred()))

		// By("checking if the custom resource was successfully created")
		Eventually(func() error {
			found := &cachev1alpha1.Memcached{}
			return k8sClient.Get(ctx, typeNamespaceName, found)
		}, config.timeout, config.interval).Should(Succeed())

		By("reconciling the custom resource created")
		memcachedReconciler := &MemcachedReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err = memcachedReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: typeNamespaceName,
		})
		Expect(err).To(Not(HaveOccurred()))

		By("checking if deployment was successfully created in the reconciliation")
		Eventually(func() error {
			found := &appsv1.Deployment{}
			return k8sClient.Get(ctx, typeNamespaceName, found)
		}, config.timeout, config.interval).Should(Succeed())

		By("checking the latest Status Condition added to the Memcached instance")
		Eventually(func() error {
			if memcached.Status.Nodes != nil && len(memcached.Status.Nodes) == int(config.Size) {
				return fmt.Errorf("memcached status nodes is not populated")
			}
			return nil
		}, config.timeout, config.interval).Should(Succeed())
	})
})
