// Copyright Broadcom, Inc. All Rights Reserved.
// SPDX-License-Identifier: APACHE-2.0

package integration

import (
	"context"
	"fmt"

	utils "github.com/bitnami/charts-private/.vib/common-tests/ginkgo-utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	cv1 "k8s.io/client-go/kubernetes/typed/core/v1"

	// For client auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// This test ensures a Kafka Producer can send a message and that message be received by a Kafka Consumer
var _ = Describe("Strimzi Kafka Operator:", func() {
	var coreclient cv1.CoreV1Interface
	var batchclient bv1.BatchV1Interface
	var ctx context.Context

	BeforeEach(func() {
		coreclient = cv1.NewForConfigOrDie(clusterConfigOrDie())
		batchclient = bv1.NewForConfigOrDie(clusterConfigOrDie())
		ctx = context.Background()
	})

	When("a Kafka cluster is created", func() {
		BeforeEach(func() {
			// We need to make sure every Kafka broker and Kafka controller pod is running before launching the tests
			kafkaPods := utils.GetPodsByLabelOrDie(ctx, coreclient, *namespace, "strimzi.io/cluster="+*kafkaClusterName)
			for _, p := range kafkaPods.Items {
				isPodRunning := false
				err := utils.Retry("IsPodRunning", 5, 5000, func() (bool, error) {
					res, err2 := utils.IsPodRunning(ctx, coreclient, *namespace, p.GetName())
					isPodRunning = res
					return res, err2
				})
				if err != nil {
					panic(err.Error())
				}
				Expect(isPodRunning).To(BeTrue())
			}
		})

		Describe("a producer should be able to send a message and a consumer should be able to receive it", func() {
			It("should create a producer job", func() {
				getSucceededJobs := func(j *batchv1.Job) int32 { return j.Status.Succeeded }

				By("obtaining a Kafka broker pod")
				pod, err := coreclient.Pods(*namespace).Get(ctx, fmt.Sprintf("%s-broker-0", *kafkaClusterName), metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())

				By("extracting the image from the Kafka broker pod")
				kafkaImage := pod.Spec.Containers[0].Image
				Expect(kafkaImage).ToNot(BeEmpty())

				By("creating a job to produce a message")
				err = createKafkaClientJob(ctx, batchclient, kafkaImage, "producer")
				Expect(err).ToNot(HaveOccurred())

				By("waiting for the job to succeed")
				Eventually(func() (*batchv1.Job, error) {
					return batchclient.Jobs(*namespace).Get(ctx, PRODUCER_JOB_NAME, metav1.GetOptions{})
				}, timeout, POLLING_INTERVAL).Should(WithTransform(getSucceededJobs, Equal(int32(1))))

				By("deleting the job once it has succeeded")
				err = batchclient.Jobs(*namespace).Delete(ctx, PRODUCER_JOB_NAME, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("creating a job to consume the message")
				err = createKafkaClientJob(ctx, batchclient, kafkaImage, "consumer")
				Expect(err).ToNot(HaveOccurred())

				By("waiting for the job to succeed")
				Eventually(func() (*batchv1.Job, error) {
					return batchclient.Jobs(*namespace).Get(ctx, CONSUMER_JOB_NAME, metav1.GetOptions{})
				}, timeout, POLLING_INTERVAL).Should(WithTransform(getSucceededJobs, Equal(int32(1))))

				By("deleting the job once it has succeeded")
				err = batchclient.Jobs(*namespace).Delete(ctx, CONSUMER_JOB_NAME, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
