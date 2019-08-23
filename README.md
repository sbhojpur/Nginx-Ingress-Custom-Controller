## Nginx-Ingress-Custom-Controller
Custom controller to create/update TCP/UDP and HTTP/s routes based on custom annotations in k8s services

We have been running our micro-services on K8s cluster and using Nginx Ingress controller to manage TCP/UDP and HTTP routes. Our Nginx controller runs as a K8s pod and all routing information is controlled by defining TCP/UDP config maps and creating/updating K8s Ingress objects. 


![TCP Configmap example](/pictures/Picture1.jpg)

This will configure Nginx Ingress controller to 
* route TCP traffic on port 1162 to mytcpapp1-svc at port 162
* route TCP traffic on port 2345 to mytcpapp2-svc at port 164

![UDP Configmap example](/pictures/Picture2.jpg)

This will configure Nginx Ingress controller to 
* route UDP traffic on port 2312 to myudpapp1-svc at port 362
* route UDP traffic on port 3312 to myudpapp2-svc at port 462

![Nginx controller configuration](/pictures/Picture3.jpg)

Nginx controller pod is configured to read 
* TCP routes from configmap 'tcp-controller-configmap'
* UDP routes from configmap 'udp-controller-configmap'


# Motivation
For every new micro-service being deployed, we have to update these configmaps or we have to create/update HTTP ingress objects. We have been looking for a way to automate this process. 
our idea is to create a custom controller which is continously watching our cluster for k8s services being created/modified or deleted and automatically updating these TCP/UDP configmaps and HTTP ingress objects.

# Solution
1. User should have a way to define TCP/UDP and HTTP routes within the service manifest file. This can be achived by defining custom annotations.
2. Create a custom controller which will..
   - Watch for k8s services being create.modified or deleted.
   - Read custom annotation from these services and update TCP Configmap, UDP configmap and HTTP/s ingress whichever is required.
   - Update port mapping information in Nginx controller service object.
   
![Custom controller](/pictures/Picture4.jpg)   

# Custom Annotation
The solution defines a new type of annotation designated as 'altranNginxIngress'. The value of this annotation is an array of JSON objects where each object has 3 fields.
* name: Name of port as defined under spec.ports section 
* route: TCP port, UDP port or HTTP URL tag
* type:  Tyep of traffic, can be TCP, UDP or HTTP
Incase TCP/UDP load balancing is required route will be the port of incoming traffic.
Incase of HTTP/s load balancing is required route will be the url tag  like '/mytag1', '/mytag2' etc. Please not character '/' is manadatory

![TCP Annotation ](/pictures/Picture5.jpg) 

Annotation 'altranNginxIngress' being used to define just one TCP route for the service.

![UDP Annotation ](/pictures/Picture6.jpg) 

Annotation 'altranNginxIngress' being used to define just one UDP route for the service.

![HTTP Annotations ](/pictures/Picture7.jpg) 

Annotation 'altranNginxIngress' being used to define 2 HTTP routes for the service.
  
# Custom Controller
* Golang based module.
* Discovers Nginx Controller pod, nginx controller service, TCP/UDP configmaps and namespaces
* Watches for K8s services being defined, modified or deleted.  
* Reads annotation 'altranNginxIngress' and collects service routing information
* Updates TCP configmap, UDP configmap and HTTP ingress objects as required.
* Update Nginx controller service for port mapping

![Nginx Custom controller ](/pictures/Picture8.jpg) 

Nginx Ingress controller expose K8s services based on the route information specified in in TCP Configmap, UDP Configmap and HTTP Ingress Objects


# Steps to run custom controller in kubenetes cluster.

Go to master node of kubernetes cluster and execute below mentioned steps:
1. Clone Nginx-Ingress-Custom-Controller repo:
   a. git clone https://github.com/sbhojpur/Nginx-Ingress-Custom-Controller.git
   b. cd Nginx-Ingress-Custom-Controller

2. Create Docker image:
   a. docker load -i /pkg/custom_controller_image.tar
   b. docker tag bdd5e0ad3bab custom_controller_image:1.1

3. Add label to master node:
   kubectl label nodes <masternode> dedicated=master

4. Create directory and place config file
   a. mkdir /opt
   b. cd /opt
   c. mkdir custom_controller_config
   d. cp /root/.kube/config /opt/custom_controller_config/
   Note: current config file path is reference to kuberspray. file location could be different.

4. Create custom controller pod using custom_controller_pod.yaml
   kubectl create -f /pkg/custom_controller_pod.yaml

3. Create service with annotation (route information, TCP config details and UDP config details)
* refer example section
  
4.	Access pod using route/TCP/UDP

