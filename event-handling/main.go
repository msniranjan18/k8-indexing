package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const NodeIndex = "byNode"

func main() {
	// 1. Build the Kubernetes Clientset using local kubeconfig
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// 2. Create the Shared Informer Factory (resyncs every 10 minutes)
	factory := informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	podInformer := factory.Core().V1().Pods()

	// 3. Add a Custom Indexer to the Informer's Cache
	// This allows O(1) lookups of Pods matching a specific Node name.
	err = podInformer.Informer().AddIndexers(cache.Indexers{
		NodeIndex: func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return nil, fmt.Errorf("not a pod")
			}
			if pod.Spec.NodeName == "" {
				return nil, nil
			}
			return []string{pod.Spec.NodeName}, nil
		},
	})
	if err != nil {
		panic(err)
	}

	// 4. Register Event Handlers (The "Observe" phase)
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			fmt.Printf("[ADD] Pod Created: %s/%s\n", pod.Namespace, pod.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*corev1.Pod)
			fmt.Printf("[UPDATE] Pod Modified: %s/%s\n", newPod.Namespace, newPod.Name)
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			fmt.Printf("[DELETE] Pod Deleted: %s/%s\n", pod.Namespace, pod.Name)
		},
	})

	// 5. Start the Informer (Runs asynchronously in the background)
	stopCh := make(chan struct{})
	defer close(stopCh)
	
	fmt.Println("Starting Pod Informer...")
	factory.Start(stopCh)

	// 6. Wait for the local cache to sync completely with the API Server
	if !cache.WaitForCacheSync(stopCh, podInformer.Informer().HasSynced) {
		panic("Timed out waiting for caches to sync")
	}
	fmt.Println("Cache synced successfully!")

	// 7. Use the Indexer to query the cache locally without hitting the API server
	// Let's query all pods assigned to a specific node named "minikube"
	go func() {
		time.Sleep(5 * time.Second) // Give it a moment to collect some data
		
		indexer := podInformer.Informer().GetIndexer()
		objs, err := indexer.ByIndex(NodeIndex, "minikube")
		if err != nil {
			fmt.Printf("Error querying index: %v\n", err)
			return
		}

		fmt.Printf("\n--- Found %d pods running on node 'minikube' via local index ---\n", len(objs))
		for _, obj := range objs {
			pod := obj.(*corev1.Pod)
			fmt.Printf("- %s/%s\n", pod.Namespace, pod.Name)
		}
	}()

	// Keep main goroutine alive
	<-stopCh
}
