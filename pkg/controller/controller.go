/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"sync"
	"time"

	netV1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// D3osGatewayController Controller demonstrates how to implement a controller with client-go.
type D3osGatewayController struct {
	ingressClassName string
	// map是必须要维护的,informer只有变更的时候才会给信息,所以一般情况下,只能通过本地信息获取ingress和service之间的关系
	ingressMap sync.Map // map[域名+path]"svc.ns:port"        ingress   处理

	ingressIndexer  cache.Indexer
	ingressInformer cache.Controller
	ingressQueue    workqueue.RateLimitingInterface
}

// NewD3osGatewayController creates a new D3osGatewayController.
func NewD3osGatewayController(c *rest.Config, ingressClassName string) *D3osGatewayController {
	// creates the clientset
	clientSet, err := kubernetes.NewForConfig(c)
	if err != nil {
		klog.Fatal(err)
	}

	// create the pod watcher
	ingressListWatcher := cache.NewListWatchFromClient(clientSet.NetworkingV1().RESTClient(), "ingresses", "", fields.Everything())

	// create the workqueue, 从informer中写入, 在syncIngressToStdout中消费
	ingressQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Bind the workqueue to a cache with the help of an informer.
	ingressIndexer, ingressInformer := cache.NewIndexerInformer(ingressListWatcher, &netV1.Ingress{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				klog.Errorf("cache.MetaNamespaceKeyFunc(obj) got an error:%s\n", err)
				return
			}
			ingressQueue.Add(&Event{
				Type:   EventAdd,
				Object: key,
			})
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err != nil {
				klog.Errorf("cache.MetaNamespaceKeyFunc(new) got an error:%s\n", err)
				return
			}
			ingressQueue.Add(&Event{
				Type:   EventUpdate,
				Object: key,
			})
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// key function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				klog.Errorf("cache.DeletionHandlingMetaNamespaceKeyFunc(obj) got an error:%s\n", err)
				return
			}
			ingressQueue.Add(&Event{
				Type:      EventDelete,
				Object:    key,
				Tombstone: obj,
			})
		},
	}, cache.Indexers{})

	return &D3osGatewayController{
		ingressClassName: ingressClassName,
		ingressMap:       sync.Map{},

		ingressIndexer:  ingressIndexer,
		ingressInformer: ingressInformer,
		ingressQueue:    ingressQueue,
	}
}

func (c *D3osGatewayController) processNextIngressItem() bool {
	// Wait until there is a new item in the working queue
	obj, quit := c.ingressQueue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.ingressQueue.Done(obj)

	// Invoke the method containing the business logic
	err := c.syncIngressToStdout(obj.(*Event))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleIngressErr(err, obj)
	return true
}

// syncToStdout is the business logic of the controller. In this controller it simply prints
// information about the pod to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (c *D3osGatewayController) syncIngressToStdout(event *Event) error {
	key := event.Object.(string)
	var ingress *netV1.Ingress
	if event.Type == EventDelete {
		ingress = event.Tombstone.(*netV1.Ingress)
	} else {
		obj, _, err := c.ingressIndexer.GetByKey(key)
		if err != nil {
			klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
			return err
		}
		ingress = obj.(*netV1.Ingress)
		if ingress == nil {
			klog.Errorf("ingress.convert.err %s ", key)
			return nil
		}
	}
	if *ingress.Spec.IngressClassName != c.ingressClassName {
		klog.Infof("ignore ingress %s, which class name is %s\n", key, ingress.Spec.IngressClassName)
		return nil
	}
	// 此处, 我们处理我们的map
	localMap := map[string]string{}
	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			tempKey := rule.Host + path.Path
			value := fmt.Sprintf("%s.%s:%d",
				path.Backend.Service.Name,
				ingress.Namespace,
				// 第一版只处理num 不处理name
				path.Backend.Service.Port.Number,
			)
			localMap[tempKey] = value
		}
	}

	// 判断删除
	if event.Type == EventDelete {
		klog.Infof("ingress删除清理本地map:%v, localMap:%v", key, localMap)
		// 遍历我们的localmap 去 ingressMap中删掉
		for k := range localMap {
			c.ingressMap.Delete(k)
		}
	} else {
		klog.Infof("ingress新增或修改 调整本地map:%v, localMap:%v", key, localMap)
		// 判断新增和修改
		for k, v := range localMap {
			c.ingressMap.Store(k, v)
		}
	}
	//rangeFunc := func(k, v interface{}) bool {
	//	klog.V(2).Infof("遍历ingressMap: %v = %v", k, v)
	//	return true
	//}
	//c.ingressMap.Range(rangeFunc)
	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *D3osGatewayController) handleIngressErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.ingressQueue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.ingressQueue.NumRequeues(key) < 5 {
		klog.Infof("Error syncing pod %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.ingressQueue.AddRateLimited(key)
		return
	}

	c.ingressQueue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	klog.Infof("Dropping pod %q out of the queue: %v", key, err)
}

// Run begins watching and syncing.
func (c *D3osGatewayController) Run(workers int, stopCh chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.ingressQueue.ShutDown()
	klog.Info("Starting Pod controller")

	go c.ingressInformer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.ingressInformer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(c.runIngressWorker, time.Second, stopCh)
	}

	<-stopCh
	klog.Info("Stopping Pod controller")
}

func (c *D3osGatewayController) runIngressWorker() {
	for c.processNextIngressItem() {
	}
}
