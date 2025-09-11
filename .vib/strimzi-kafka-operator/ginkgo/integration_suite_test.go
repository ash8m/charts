// Copyright Broadcom, Inc. All Rights Reserved.
// SPDX-License-Identifier: APACHE-2.0

package integration

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	// For client auth plugins

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	APP_NAME          = "Strimzi Kafka Operator"
	PRODUCER_JOB_NAME = "kafka-producer"
	CONSUMER_JOB_NAME = "kafka-consumer"
	KAFKA_TOPIC       = "test-topic"
	POLLING_INTERVAL  = 1 * time.Second
)

var (
	kubeconfig       = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	namespace        = flag.String("namespace", "", "namespace where the resources are deployed")
	kafkaClusterName = flag.String("kafka-cluster-name", "my-cluster", "name of the Kafka cluster")
	timeoutSeconds   = flag.Int("timeout", 120, "timeout in seconds")
	timeout          time.Duration
)

func init() {
	timeout = time.Duration(*timeoutSeconds) * time.Second
}

func clusterConfigOrDie() *rest.Config {
	var config *rest.Config
	var err error

	if *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		panic(err.Error())
	}

	return config
}

func createKafkaClientJob(ctx context.Context, c bv1.BatchV1Interface, image, role string) error {
	securityContext := &v1.SecurityContext{
		Privileged:               &[]bool{false}[0],
		AllowPrivilegeEscalation: &[]bool{false}[0],
		RunAsNonRoot:             &[]bool{true}[0],
		Capabilities: &v1.Capabilities{
			Drop: []v1.Capability{"ALL"},
		},
		SeccompProfile: &v1.SeccompProfile{
			Type: "RuntimeDefault",
		},
	}

	var command string
	var jobName string
	switch role {
	case "producer":
		command = fmt.Sprintf("echo 'foo' | bin/kafka-console-producer.sh --bootstrap-server %s-kafka-bootstrap:9092 --topic %s", *kafkaClusterName, KAFKA_TOPIC)
		jobName = PRODUCER_JOB_NAME
	case "consumer":
		command = fmt.Sprintf("bin/kafka-console-consumer.sh --bootstrap-server %s-kafka-bootstrap:9092 --topic %s --from-beginning --max-messages 1 | grep foo", *kafkaClusterName, KAFKA_TOPIC)
		jobName = CONSUMER_JOB_NAME
	default:
		return fmt.Errorf("unknown role: %s", role)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Job",
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy: "Never",
					Containers: []v1.Container{
						{
							Name:       "kafka",
							Image:      image,
							WorkingDir: "/opt/kafka",
							Command:    []string{"bash"},
							Args: []string{
								"-c", command,
							},
							SecurityContext: securityContext,
						},
					},
				},
			},
		},
	}

	_, err := c.Jobs(*namespace).Create(ctx, job, metav1.CreateOptions{})

	return err
}

func CheckRequirements() {
	if *namespace == "" {
		panic(fmt.Sprintf("The namespace where %s is deployed must be provided. Use the '--namespace' flag", APP_NAME))
	}
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, fmt.Sprintf("%s Integration Tests", APP_NAME))
}
