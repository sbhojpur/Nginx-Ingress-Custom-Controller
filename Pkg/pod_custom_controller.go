package main

import (

    "fmt"
    "time"
    "os"
    "path/filepath"
    "k8s.io/klog"
    "strconv"
    "encoding/json"
    "strings"
    "reflect"
    "log"
    "k8s.io/client-go/util/workqueue"
    meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/fields"
    "k8s.io/client-go/kubernetes"
    "k8s.io/apimachinery/pkg/util/runtime"
    "k8s.io/client-go/tools/cache"
    "k8s.io/api/extensions/v1beta1"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/util/wait"
    "k8s.io/apimachinery/pkg/util/intstr"

)

type Controller struct {
    indexer    cache.Indexer
    queue      workqueue.RateLimitingInterface
    informer   cache.Controller
    clientset  *kubernetes.Clientset
}

type HostTagServPortData struct {
    host        string
    tag         string
    serv_Name    string
    serv_Port    int
}


func custom_NewController(queue1 workqueue.RateLimitingInterface, indexer1 cache.Indexer, informer1 cache.Controller, clientset1 *kubernetes.Clientset) *Controller {
    return &Controller{
        informer:  informer1,
        indexer:   indexer1,
        queue:     queue1,
        clientset: clientset1,
    }
}




func (c *Controller) custom_controller_processNextItem() bool {

    key1, quit := c.queue.Get()
    if quit {
        klog.Info("Returning false")
        return false
    }
    defer c.queue.Done(key1)

    err := c.custom_syncToStdout(key1.(string))

    c.handleErr(err, key1)
    return true
}



func readUpdNginxControllerServPortMap(clientset *kubernetes.Clientset, nginxTCPConfigMapData map[string]string, nginxUDPConfigMapData map[string]string ) {

    var nginxContServPortMap  = make(map[int]v1.ServicePort)
    var servicePort v1.ServicePort
    var nginxContServ v1.Service
    //Find Nginx controller serivce
    services, err := clientset.CoreV1().Services("").List(meta_v1.ListOptions{})
    if err != nil {
        klog.Fatal(err)
    }
    for _, serv := range services.Items {
        if serv.GetLabels()["app"] == "nginx-ingress" && serv.GetLabels()["component"] == "controller" {
           nginxContServ = serv
           break
        }
    }
    if nginxContServ.GetName() == "" {
       err := "Nginx controller service not found!"
       klog.Info(err)
       klog.Fatal(err)
    } else {
       for _, specPortArrElem :=  range nginxContServ.Spec.Ports {
          nginxContServPortMap[specPortArrElem.TargetPort.IntValue()] = specPortArrElem
       }
    }

    nginxContServ.Spec.Ports = nginxContServ.Spec.Ports[:2]

    for targetPort, mapVal := range nginxTCPConfigMapData {
       servicePort.Name = "tcpport"+targetPort
       servicePort.Protocol = "TCP"
       port,_  := strconv.Atoi(targetPort)
       servicePort.Port = int32(port)
       port, _ = strconv.Atoi(strings.Split(mapVal,":")[1])
       servicePort.TargetPort = intstr.FromInt(port)
       nginxContServ.Spec.Ports = append(nginxContServ.Spec.Ports, servicePort)
    }

    for targetPort, mapVal := range nginxUDPConfigMapData {
       servicePort.Name = "udpport"+targetPort
       servicePort.Protocol = "UDP"
       port, _ := strconv.Atoi(targetPort)
       servicePort.Port = int32(port)
       port, _ = strconv.Atoi(strings.Split(mapVal,":")[1])
       servicePort.TargetPort = intstr.FromInt(port)
       nginxContServ.Spec.Ports = append(nginxContServ.Spec.Ports, servicePort)
    }

    klog.Info("TCPMAP: ", nginxTCPConfigMapData)
    klog.Info("UDPMAP: ", nginxUDPConfigMapData)
    nginxContServ.Spec.Type = "NodePort"
    _ , errserv := clientset.CoreV1().Services(nginxContServ.GetNamespace()).Update(&nginxContServ)

    if errserv != nil {
        klog.Info("Cannot update Nginx controller service Port mappping. Error is: ",errserv)
    }
}


func getNginxIngressPathData(clientset *kubernetes.Clientset, key string) (map[string]HostTagServPortData, v1beta1.Ingress)  {
    var nginxHttpIngressData = make(map[string]HostTagServPortData)
    var nginxHttpIngress v1beta1.Ingress
    var namespace string = strings.Split(key, "/")[0]
    var nginxHTTPIngressExist bool = false

    //Find http ingress for Nginx configuration
    ingresses, err := clientset.ExtensionsV1beta1().Ingresses(namespace).List(meta_v1.ListOptions{})
    if err != nil {
        klog.Fatal(err)
    }

    for _, ing := range ingresses.Items {
        if ing.GetName() == "altran-nginx-ingress" {
            nginxHTTPIngressExist = true
            nginxHttpIngress = ing
        }
    }


    if !nginxHTTPIngressExist {
       nginxHttpIngress.SetName("altran-nginx-ingress")
       nginxHttpIngress.SetNamespace(namespace)
       nginxHttpIngress.SetAnnotations(map[string]string{"kubernetes.io/ingress.class":"nginx", "nginx.ingress.kubernetes.io/ssl-redirect":"false", "nginx.ingress.kubernetes.io/auth-tls-verify-client": "on"})
       nginxHttpIngress.Spec.Rules = []v1beta1.IngressRule{v1beta1.IngressRule{"dummy.com",
              v1beta1.IngressRuleValue{&v1beta1.HTTPIngressRuleValue{[]v1beta1.HTTPIngressPath{v1beta1.HTTPIngressPath{"/dummytag",v1beta1.IngressBackend{"dummyservice",intstr.FromInt(65535)}}}}} }}
       _, err := clientset.ExtensionsV1beta1().Ingresses(namespace).Create(&nginxHttpIngress)
        if err != nil {
           klog.Info("Cannot create Nginx Ingress forn namespace",namespace,err)
       }
    }

    for _, rule := range nginxHttpIngress.Spec.Rules {
       if rule.Host == "" {
          rule.Host = "HostAny" }
       for _, path := range rule.HTTP.Paths {
          nginxHttpIngressData[rule.Host+path.Path+"/"+path.Backend.ServiceName] =
            HostTagServPortData{rule.Host, path.Path, path.Backend.ServiceName, path.Backend.ServicePort.IntValue() }
       }
    }
    return nginxHttpIngressData, nginxHttpIngress
}



func getNginxConfigMapData(clientset *kubernetes.Clientset) (v1.ConfigMap, map[string]string, v1.ConfigMap, map[string]string) {

    var nginxTCPConfigMapData = make(map[string]string)
    var nginxUDPConfigMapData = make(map[string]string)
    var nginxTCPConfigMap v1.ConfigMap
    var nginxUDPConfigMap v1.ConfigMap
    var nginxIngressDeployment v1beta1.Deployment
    var tcpConfigMapidx int = -1
    var udpConfigMapidx int = -1

    //Update TCP/UDP ConfigMap entries if not exists in Nginx controller pod
    klog.Info("Update TCP/UDP ConfigMap entries if not exists in Nginx controller pod")
    deployments, err := clientset.ExtensionsV1beta1().Deployments("").List(meta_v1.ListOptions{})
    if err != nil {
        klog.Fatal(err)
    }
    for _, deployment := range deployments.Items {
        if deployment.GetLabels()["app"] == "nginx-ingress" && deployment.GetLabels()["component"] == "controller" {
           nginxIngressDeployment = deployment
           break
        }
    }
    if nginxIngressDeployment.GetName() == "" {
       klog.Info("Nginx ingress controller deployment not found")
       errdep :="Nginx ingress controller deployment not found"
       klog.Info(errdep)
    } else {
       klog.Info("Arguments: ",nginxIngressDeployment.Spec.Template.Spec.Containers[0].Args)
       for idx, val := range nginxIngressDeployment.Spec.Template.Spec.Containers[0].Args {
          if strings.Contains(val, "--tcp-services-configmap") {
             tcpConfigMapidx = idx
          }
          if strings.Contains(val, "--udp-services-configmap") {
             udpConfigMapidx = idx
          }
       }
       klog.Info("value of tcpConfigMapidx and udpConfigMapidx are:", tcpConfigMapidx, udpConfigMapidx )
       if tcpConfigMapidx == -1 {
          klog.Info("\nupdating --tcp-services-configmap to be updated in nginx deployment\n")
          nginxIngressDeployment.Spec.Template.Spec.Containers[0].Args = append(nginxIngressDeployment.Spec.Template.Spec.Containers[0].Args,
              "--tcp-services-configmap="+nginxIngressDeployment.GetNamespace()+"/tcp-controller-configmap")
       }
       if udpConfigMapidx == -1 {
          klog.Info("\nupdating --udp-services-configmap to be updated in nginx deployment\n")
          nginxIngressDeployment.Spec.Template.Spec.Containers[0].Args = append(nginxIngressDeployment.Spec.Template.Spec.Containers[0].Args,
              "--udp-services-configmap="+nginxIngressDeployment.GetNamespace()+"/udp-controller-configmap")
       }
       if tcpConfigMapidx == -1 || udpConfigMapidx == -1 {
          klog.Info("\n Updating ingress deployment")
          _ , errdep := clientset.ExtensionsV1beta1().Deployments(nginxIngressDeployment.GetNamespace()).Update(&nginxIngressDeployment)
          if errdep != nil {
             klog.Info(errdep)
          }

       }
    }

    //Find Configmap for Nginx configuration
    klog.Info("\nFind Configmap for Nginx configuration\n")
    configMaps, err := clientset.CoreV1().ConfigMaps("").List(meta_v1.ListOptions{})
    if err != nil {
        klog.Fatal(err)
    }
    for _, cm := range configMaps.Items {
        if cm.GetName() == "tcp-controller-configmap" {
            nginxTCPConfigMap = cm
            nginxTCPConfigMapData = cm.Data
            break
        }
    }
    for _, cm := range configMaps.Items {
        if cm.GetName() == "udp-controller-configmap" {
            nginxUDPConfigMap = cm
            nginxUDPConfigMapData = cm.Data
            break
        }
    }

    //if tcp controller configmap doesn't exist, we will create one

    klog.Info("\n check if tcp controller configmap doesn't exist, we will create one\n")
    if nginxTCPConfigMap.GetName() == "" {
       nginxTCPConfigMap.SetName("tcp-controller-configmap")
       nginxTCPConfigMap.SetNamespace(nginxIngressDeployment.GetNamespace())
       nginxTCPConfigMap.Data = make(map[string]string)
       nginxTCPConfigMap.Data["65534"] = "default/dummy-service:80"
       _, err := clientset.CoreV1().ConfigMaps(nginxTCPConfigMap.GetNamespace()).Create(&nginxTCPConfigMap)
        if err != nil {
           klog.Info("Cannot create TCP confifMap for Nginx ingress")
        }
    }

    //if udp controller configmap doesn't exist, we will create one
    klog.Info("\ncheck if udp controller configmap doesn't exist, we will create one\n")
    if nginxUDPConfigMap.GetName() == "" {
       nginxUDPConfigMap.SetName("udp-controller-configmap")
       nginxUDPConfigMap.SetNamespace(nginxIngressDeployment.GetNamespace())
       nginxUDPConfigMap.Data = make(map[string]string)
       nginxUDPConfigMap.Data["65534"] = "default/dummy-service:80"
       _, err := clientset.CoreV1().ConfigMaps(nginxUDPConfigMap.GetNamespace()).Create(&nginxUDPConfigMap)
        if err != nil {
           klog.Info("Cannot create UDP confifMap for Nginx ingress")
        }
    }

    klog.Info("New code:Remove route information from nginx ingress")


    return nginxTCPConfigMap, nginxTCPConfigMapData, nginxUDPConfigMap, nginxUDPConfigMapData
}

func (c *Controller) custom_syncToStdout(key string) error {
    var action string  = key[ len(key) - 3 :]
    key = key[: len(key) - 4]
    obj, exists, err := c.indexer.GetByKey(key)
    var servicePortArr [10]v1.ServicePort
    var annotRouteInfoArr []map[string]string
    var updUDPConfigMapData = make(map[string]string)
    var updTCPConfigMapData = make(map[string]string)
    var updHTTPIngressData  = make(map[string]HostTagServPortData)
    var updProtocol int = 0 //TCP 1, UDP 2, HTTP 4, WebSocket 8

    var nginxTCPConfigMapData map[string]string
    var nginxUDPConfigMapData map[string]string
    var nginxTCPConfigMap v1.ConfigMap
    var nginxUDPConfigMap v1.ConfigMap
    var nginxHTTPIngressData map[string]HostTagServPortData
    var nginxHTTPIngress v1beta1.Ingress
    var ifUpdateConfig  bool = false

    if err != nil {
        klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
    return err
    }

    if !exists {
        // Custom controller: Below we will warm up our cache with a Pod, so that we will see a delete for one pod
        log.Printf("\nService %s does not exist anymore\n", key)
        //Find Configmap for Nginx configuration
        klog.Info("\nFind Configmap for Nginx configuration\n")
        nginxTCPConfigMap, nginxTCPConfigMapData, nginxUDPConfigMap, nginxUDPConfigMapData = getNginxConfigMapData(c.clientset)

        //Remove routing info for deleted service
        klog.Info("\nRemove routing info for deleted service\n")
        if nginxTCPConfigMap.GetName() != ""{
            for port, _ := range nginxTCPConfigMapData {
                if strings.Contains(nginxTCPConfigMapData[port], key) {
                    if _, ok :=  updTCPConfigMapData[port]; !ok {
                        delete(nginxTCPConfigMapData, port)
                        ifUpdateConfig = true
                        updProtocol = updProtocol | 1
                    }
                }
            }
        }
        //Remove deleted entry from Nginx config map data
        klog.Info("\nRemove deleted entry from Nginx config map data\n")
        if nginxUDPConfigMap.GetName() != ""{
            for port, _ := range nginxUDPConfigMapData {
                if strings.Contains(nginxUDPConfigMapData[port], key) {
                    if _, ok :=  updUDPConfigMapData[port]; !ok {
                        delete(nginxUDPConfigMapData, port)
                        ifUpdateConfig = true
                        updProtocol = updProtocol | 2
                    }
                }
            }
        }
        nginxHTTPIngressData, nginxHTTPIngress = getNginxIngressPathData(c.clientset, key)
        if nginxHTTPIngress.GetName() != "" {
           //Remove non existent routes
           klog.Info("\nRemove non existent routes\n")
           klog.Info("nginxHTTPIngress:", nginxHTTPIngress)
           klog.Info("INGRESDATA: ", nginxHTTPIngressData)
           klog.Info("UPDINGRESDATA: ", updHTTPIngressData)
           klog.Info("key:namespaces/service value is :", key)
           var servicename string
           if strings.Contains(key, "/") {
           servicename = strings.Split(key, "/")[1]
           }
           for nginxHTTPIngressDataMapKey, _  := range nginxHTTPIngressData {
              klog.Info("nginxHTTPIngressDataMapKey is:",nginxHTTPIngressDataMapKey)
              if strings.Contains(nginxHTTPIngressDataMapKey, servicename){
               //if _, ok := updHTTPIngressData[nginxHTTPIngressDataMapKey]; !ok {
                 klog.Info("delete route nginx ingress")
                 delete(nginxHTTPIngressData, nginxHTTPIngressDataMapKey)
                 ifUpdateConfig = true
                 updProtocol = updProtocol | 4
              }
           }
        }
    } else {
         klog.Info("Execute when service is created")
        //Check if service has Nginx annotation
        if val, ok :=  (obj.(*v1.Service).GetAnnotations())["altranNginxIngress"]; ok {
            unmarErr := json.Unmarshal([]byte(val), &annotRouteInfoArr)
            klog.Info("printing value of unmarErr")
            klog.Info(unmarErr)
            if unmarErr != nil {
                klog.Info("Cannot fetch routing info from nginx annotation")
                return unmarErr
            }

            klog.Info("\nHandling service: ",key)

            //Find Configmap for Nginx configuration and read configmap data
            nginxTCPConfigMap, nginxTCPConfigMapData, nginxUDPConfigMap, nginxUDPConfigMapData = getNginxConfigMapData(c.clientset)


            //Find Ingress fro Nginx Http and read http route data
            klog.Info("\nFind Ingress fro Nginx Http and read http route data")
            nginxHTTPIngressData, nginxHTTPIngress = getNginxIngressPathData(c.clientset, key)

            klog.Info(nginxHTTPIngressData,nginxHTTPIngress.GetName())
            //Read all port mapping info from service spec
            klog.Info("Read all port mapping info from service spec")
            for idx:=0; idx < len(obj.(*v1.Service).Spec.Ports); idx++ {
                servicePortArr[idx] = obj.(*v1.Service).Spec.Ports[idx]
            }
            //Read service port mapping info and generate new routing info nginx
            klog.Info("Read service port mapping info and generate new routing info nginx")
            klog.Info("value of annotRouteInfoArr is: ",annotRouteInfoArr)
            klog.Info("Validate annotations defined")
            //count :=0
            for annot_idx:=0; annot_idx < len(annotRouteInfoArr); annot_idx++ {
                count :=0
                for annot_jdx:=0; annot_jdx < len(annotRouteInfoArr); annot_jdx++ {
                    if (annotRouteInfoArr[annot_idx])["route"] ==  (annotRouteInfoArr[annot_jdx])["route"] {
                        count++
                     }
                }
                    if count !=1 {
                        err := "\n Service yaml containes duplicate value"
                        klog.Info(err)
                        return nil
                    } else {
                         klog.Info("Service yaml validated successfully")
                    }
             }
            //count :=0
            for port_idx:=0; port_idx < len(obj.(*v1.Service).Spec.Ports); port_idx++ {
                    if (servicePortArr[port_idx].Name ==  "") {
                        err := "\n service port named is not given"
                        klog.Info(err)
                        return nil
                     }
             }


            //count :=0
            for port_idx:=0; port_idx < len(obj.(*v1.Service).Spec.Ports); port_idx++ {
                    //klog.Info("service protocol is :", servicePortArr[port_idx].Protocol )
                    if (servicePortArr[port_idx].Protocol ==  "") {
                        err := "\n service protocol is not given"
                        klog.Info(err)
                        return nil
                     }
             }

            for annot_idx:=0; annot_idx < len(annotRouteInfoArr); annot_idx++ {
                for port_idx:=0; port_idx < len(obj.(*v1.Service).Spec.Ports); port_idx++ {
                    if (annotRouteInfoArr[annot_idx])["name"] ==  servicePortArr[port_idx].Name {
                        if (annotRouteInfoArr[annot_idx])["type"] == "TCP" {
                            if servicePortArr[port_idx].Protocol == "TCP" {
                                updTCPConfigMapData[annotRouteInfoArr[annot_idx]["route"]] = obj.(*v1.Service).GetNamespace()+"/"+
                                obj.(*v1.Service).GetName() +
                                ":" +strconv.Itoa(int(servicePortArr[port_idx].Port))
                                updProtocol = updProtocol | 1
                            } else {
                                err := "\nMismatch Protocol configurations. Annotations defines TCP and spec.port.protocol is "+ servicePortArr[port_idx].Protocol
                                klog.Info(err)
                                return nil
                            }
                        }
                        if (annotRouteInfoArr[annot_idx])["type"] == "UDP" {
                            if servicePortArr[port_idx].Protocol == "UDP" {
                                updUDPConfigMapData[annotRouteInfoArr[annot_idx]["route"]] = obj.(*v1.Service).GetNamespace()+"/"+
                                obj.(*v1.Service).GetName() +
                                ":" +strconv.Itoa(int(servicePortArr[port_idx].Port))
                                updProtocol = updProtocol | 2
                            } else {
                                err := "Mismatch Protocol configurations. Annotations defines UDP and spec.port.protocol is "+ servicePortArr[port_idx].Protocol
                                klog.Info(err)
                                return nil
                            }
                        }
                        if (annotRouteInfoArr[annot_idx])["type"] == "HTTP" {
                            if servicePortArr[port_idx].Protocol == "TCP" {
                                if annotRouteInfoArr[annot_idx]["host"] == "" {
                                   annotRouteInfoArr[annot_idx]["host"] = "HostAny" }
                                updHTTPIngressData[annotRouteInfoArr[annot_idx]["host"] + annotRouteInfoArr[annot_idx]["route"] + "/"+ obj.(*v1.Service).GetName() ] = 
                                  HostTagServPortData{ annotRouteInfoArr[annot_idx]["host"], annotRouteInfoArr[annot_idx]["route"], obj.(*v1.Service).GetName(), int(servicePortArr[port_idx].Port) }
                                updProtocol = updProtocol | 4
                                klog.Info("\nHERE: ",updHTTPIngressData)
                            } else {
                                err := "Mismatch Protocol configurations. Annotations defines HTTP/TCP and spec.port.protocol is "+ servicePortArr[port_idx].Protocol
                                klog.Info(err)
                                return nil
                           }
                        }
                    }
                }
            }

            switch action {
                case "add":
                {
                    if updProtocol & 1 > 0 {
                        if nginxTCPConfigMap.GetName() != ""{
                            for port, _ := range updTCPConfigMapData {
                                if _, ok :=  nginxTCPConfigMapData[port]; ok {
                                    if updTCPConfigMapData[port] != nginxTCPConfigMapData[port] {
                                       err := "\nTCP Port mapping: "+port+": "+ nginxTCPConfigMapData[port]+" already exists"
                                       klog.Info(err)
                                       return nil
                                    }
                                } else {
                                    nginxTCPConfigMapData[port] = updTCPConfigMapData[port]
                                    ifUpdateConfig = true
                                }
                            }
                        }
                    }

                    if updProtocol & 2 > 0 {
                        if nginxUDPConfigMap.GetName() != "" {
                            for port, _ := range updUDPConfigMapData {
                                if _, ok :=  nginxUDPConfigMapData[port]; ok {
                                    if nginxUDPConfigMapData[port] != nginxUDPConfigMapData[port] {
                                       err := "\nUDP Port mapping: "+port+": "+ nginxUDPConfigMapData[port]+" already exists"
                                       klog.Info(err)
                                       return nil
                                    }
                                }else {
                                    nginxUDPConfigMapData[port] = updUDPConfigMapData[port]
                                    ifUpdateConfig = true
                                }
                            }
                        }
                    }
                    if updProtocol & 4 > 0 {
                        klog.Info("\nINGRESS NAME ", nginxHTTPIngress.GetName())
                        if nginxHTTPIngress.GetName() != "" {
                           for updHTTPIngressDataMapKey, updHTTPIngressDataMapVal := range updHTTPIngressData {
                              for _, key := range reflect.ValueOf(nginxHTTPIngressData).MapKeys() {
                                 if strings.Contains(key.String(), updHTTPIngressDataMapVal.tag) {
                                    klog.Info(" \nHttp route  ",updHTTPIngressDataMapVal.tag," already used. Old route is ", key.String())
                                    return nil
                                 }
                              }
                              if _, ok := nginxHTTPIngressData[updHTTPIngressDataMapKey]; !ok {
                                 nginxHTTPIngressData[updHTTPIngressDataMapKey] =  updHTTPIngressDataMapVal
                                 ifUpdateConfig = true
                              } else {
                                 klog.Info(" \nRoute ",updHTTPIngressDataMapVal ,"already used in Ingress")
                                 return nil
                              }
                           }
                        }
                    }
                }
                case "upd":
                {
                    if updProtocol & 1 > 0 {
                        if nginxTCPConfigMap.GetName() != ""{
                            for port, _ := range updTCPConfigMapData {
                                if _, ok :=  nginxTCPConfigMapData[port]; ok {
                                    //Check if the ingress port is already assigned to some other service
                                    if strings.Contains(updTCPConfigMapData[port], strings.TrimRight(nginxTCPConfigMapData[port],":"+port)) {
                                        nginxTCPConfigMapData[port] = updTCPConfigMapData[port]
                                        ifUpdateConfig = true
                                    } else {
                                        err := "\nConflicting TCP Port mapping: "+port+": "+ nginxTCPConfigMapData[port]
                                        klog.Info(err)
                                        return nil
                                    }
                                } else {
                                    nginxTCPConfigMapData[port] = updTCPConfigMapData[port]
                                    ifUpdateConfig = true
                                }
                            }
                            //Remove deleted entry from Nginx config map data
                            for port, _ := range nginxTCPConfigMapData {
                                if strings.Contains(nginxTCPConfigMapData[port], obj.(*v1.Service).GetNamespace()+"/"+
                                    obj.(*v1.Service).GetName()) {
                                    if _, ok :=  updTCPConfigMapData[port]; !ok {
                                        delete(nginxTCPConfigMapData, port)
                                        ifUpdateConfig = true
                                    }
                                }
                            }
                        }
                    }

                    if updProtocol & 2 > 0 {
                        if nginxUDPConfigMap.GetName() != "" {
                            for port, _ := range updUDPConfigMapData {
                                if _, ok :=  nginxUDPConfigMapData[port]; ok {
                                    if strings.Contains(updUDPConfigMapData[port], strings.TrimRight(nginxUDPConfigMapData[port],":"+port)) {
                                        err := "\nConflicting UDP Port mapping: "+port+": "+ nginxUDPConfigMapData[port]
                                        klog.Info(err)
                                        return nil
                                    } else {
                                        nginxUDPConfigMapData[port] = updUDPConfigMapData[port]
                                        ifUpdateConfig = true
                                    }
                                } else {
                                    nginxUDPConfigMapData[port] = updUDPConfigMapData[port]
                                    ifUpdateConfig = true
                                }
                            }
                            //Remove deleted entry from Nginx config map data
                            for port, _ := range nginxUDPConfigMapData {
                                if strings.Contains(nginxUDPConfigMapData[port], obj.(*v1.Service).GetNamespace()+"/"+
                                    obj.(*v1.Service).GetName()) {
                                    if _, ok :=  updUDPConfigMapData[port]; !ok {
                                        delete(nginxUDPConfigMapData, port)
                                        ifUpdateConfig = true
                                    }
                                }
                            }
                        }
                    }
                    if updProtocol & 4 > 0 {
                        if nginxHTTPIngress.GetName() != "" {
                           for updHTTPIngressDataMapKey, updHTTPIngressDataMapVal := range updHTTPIngressData {
                              if _, ok := nginxHTTPIngressData[updHTTPIngressDataMapKey]; !ok {
                                 nginxHTTPIngressData[updHTTPIngressDataMapKey] =  updHTTPIngressDataMapVal
                                 ifUpdateConfig = true
                              } else {
                                 klog.Info("\n Route updHTTPIngressDataMapVal already used in Ingress")
                                 return nil
                              }
                           }
                           //Remove non existent routes
                           for nginxHTTPIngressDataMapKey, _  := range nginxHTTPIngressData {
                              if _, ok := updHTTPIngressData[nginxHTTPIngressDataMapKey]; !ok {
                                 delete(nginxHTTPIngressData, nginxHTTPIngressDataMapKey)
                                 ifUpdateConfig = true
                              }
                           }

                        }
                    }

                }
                case "del":
                {
                    klog.Info("\nNginx routing info removed for service: ",key)
                    klog.Info("start the new program")
                }
                //case "default"
            }
        }
    }
    if ifUpdateConfig && updProtocol & 1 > 0  {
        nginxTCPConfigMap.Data = nginxTCPConfigMapData
        _, err := c.clientset.CoreV1().ConfigMaps("default").Update(&nginxTCPConfigMap)
        if err != nil {
           klog.Info("\nCannot update TCP data for Nginx")
        }
    }
    if ifUpdateConfig && updProtocol & 2 > 0 {
        nginxUDPConfigMap.Data = nginxUDPConfigMapData
        _, err := c.clientset.CoreV1().ConfigMaps("default").Update(&nginxUDPConfigMap)
        if err != nil {
           klog.Info("\nCannot update UDP data for Nginx")
        }
    }

    if ifUpdateConfig && updProtocol & 3 > 0 {
       readUpdNginxControllerServPortMap(c.clientset, nginxTCPConfigMapData, nginxUDPConfigMapData)
    }
    var ruleDataArr []v1beta1.IngressRule
    var ruleDataElem v1beta1.IngressRule
    var host string
    var path v1beta1.HTTPIngressPath
    var pathArrMap = map[string][]v1beta1.HTTPIngressPath{}
    var ifHostExist bool

    namespace := strings.Split(key, "/")[0]
    if ifUpdateConfig && updProtocol & 4 > 0 {
        klog.Info("\n UPDATE INGRESS")
        nginxHTTPIngressDataMapKeys := reflect.ValueOf(nginxHTTPIngressData).MapKeys()
        for _, key := range(nginxHTTPIngressDataMapKeys) {
            host = strings.Split(key.String(),"/")[0]
            ifHostExist  = false
            if ruleDataArr != nil {
               for _, val := range ruleDataArr {
                 if val.Host == host {
                    ifHostExist = true
                 }
               }
            }
            if !ifHostExist {
               ruleDataElem.Host = host
               ruleDataArr = append(ruleDataArr,ruleDataElem)
               pathArrMap[ruleDataElem.Host] = nil
            }
        }
        for nginxHTTPIngressDataMapKey, nginxHTTPIngressDataMapVal := range nginxHTTPIngressData {
           host = strings.Split(nginxHTTPIngressDataMapKey,"/")[0]
           path.Path = nginxHTTPIngressDataMapVal.tag
           path.Backend.ServiceName = nginxHTTPIngressDataMapVal.serv_Name
           path.Backend.ServicePort = intstr.FromInt(nginxHTTPIngressDataMapVal.serv_Port)
           pathArrMap[host] = append(pathArrMap[host], path) 
        }
        for idx, _ := range ruleDataArr {
           ruleDataArr[idx].HTTP = &v1beta1.HTTPIngressRuleValue{pathArrMap[ruleDataArr[idx].Host]}
           if ruleDataArr[idx].Host == "HostAny" {
              ruleDataArr[idx].Host = ""
           }
        }
        nginxHTTPIngress.Spec.Rules = ruleDataArr
        _, err := c.clientset.ExtensionsV1beta1().Ingresses(namespace).Update(&nginxHTTPIngress)
        if err != nil {
           klog.Info("\nCannot update Nginx Ingress:",err)
        }
    }


    return nil
}

// Custom controller: handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(Error error, key interface{}) {
    key1:= key
    if Error == nil {

        c.queue.Forget(key1)
        return
    }

    // Custom controller: controller will try will  5 times if something wrong. After, it stops trying it.

    if c.queue.NumRequeues(key1) <= 4 {
        klog.Infof("Error syncing pod %v: %v", key1, Error)


        c.queue.AddRateLimited(key1)
        return
    }

    c.queue.Forget(key1)

    runtime.HandleError(Error)
    klog.Infof("Drop pod %q out of the avaible queue: %v", key1, Error)
}

func (c *Controller) Run(threadiness int, stopCh chan struct{}) {
    defer runtime.HandleCrash()

    // Custom controller: Let the workers stop when we are done
    defer c.queue.ShutDown()
    klog.Info("Custom controller: Starting Service controller")

    go c.informer.Run(stopCh)

    // Custom controller: Wait for all involved caches to be synced, before processing items from the queue is started
    if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
        runtime.HandleError(fmt.Errorf("Custom controller: Timed out waiting for caches to sync"))
        return
    }

    for j := 0; j < threadiness; j++ {
        // Custom controller
        go wait.Until(c.runWorker, time.Second, stopCh)
    }

    <-stopCh
    klog.Info("Stopping service controller")
}

func (c *Controller) runWorker() {
    for c.custom_controller_processNextItem() {
    }
}

func main() {
    klog.InitFlags(nil)
    kubeconfig := filepath.Join(
                os.Getenv("HOME"), ".kube", "config",
        )
        // Custom controller: creates the connection
    config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
    //klog.Info("config: ", config)
        if err != nil {
                klog.Info(err)
        klog.Fatal(err)
    }

    // Custom controller: creates the clientset
    clientset, err := kubernetes.NewForConfig(config)

    if err != nil {
                klog.Info(err)
        klog.Fatal(err)
    }

        // Custom controller: create the service watcher
        svcListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "services","" , fields.Everything())

    // Custom controller: create the workqueue
    queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

    // Custom controller: Bind the workqueue to a cache with the help of an informer. This way we make sure that
    // Custom controller: whenever the cache is updated, the pod key is added to the workqueue.
    // Custom controller: Note that when we finally process the item from the workqueue, we might see a newer version
    // Custom controller: of the Pod than the version which was responsible for triggering the update.
    // Custom controller: indexer, informer := cache.NewIndexerInformer(svcListWatcher, &v1.Pod{}, 0, cache.ResourceEventHandlerFuncs{

        indexer, informer := cache.NewIndexerInformer(svcListWatcher, &v1.Service{}, 0, cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
            key, err := cache.MetaNamespaceKeyFunc(obj)
            if err == nil {
                queue.Add(key+"/add")
            }
        },
        UpdateFunc: func(old interface{}, new interface{}) {
            key, err := cache.MetaNamespaceKeyFunc(new)
            if err == nil {
                queue.Add(key+"/upd")
            }
        },
        DeleteFunc: func(obj interface{}) {
            // Custom controller: IndexerInformer uses a delta queue, therefore for deletes we have to use this
            // Custom controller: key function.
            key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
            klog.Info("KEY IN CASE OF DELETION: ", key)
            if err == nil {
                queue.Add(key+"/del")
            }
        },
    }, cache.Indexers{})

    controller := custom_NewController(queue, indexer, informer, clientset)


    // Custom controller: start
    stop := make(chan struct{})
    defer close(stop)
    go controller.Run(1, stop)

    // Custom controller: Wait
    select {}
}
