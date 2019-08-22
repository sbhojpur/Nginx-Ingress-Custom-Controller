## Nginx-Ingress-Custom-Controller
Custom controller to create/update TCP/UDP and HTTP/s routes based on custom annotations in k8s services

We have been running our micro-services on K8s cluster and using Nginx Ingress controller to manage TCP/UDP and HTTP routes. Our Nginx controller runs as a K8s pod and all routing information is controlled by defining TCP/UDP config maps and creating/updating K8s Ingress objects. 


![TCP Configmap example](/pictures/Picture1.png)

This will configure Nginx Ingress controller to 
* route TCP traffic on port 1162 to mytcpapp1-svc at port 162
* route TCP traffic on port 2345 to mytcpapp2-svc at port 164

![UDP Configmap example](/pictures/Picture2.png)

This will configure Nginx Ingress controller to 
* route UDP traffic on port 2312 to myudpapp1-svc at port 362
* route UDP traffic on port 3312 to myudpapp2-svc at port 462

![Nginx controller configuration](/pictures/Picture3.png)

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

Annotation 'altranNginxIngress' being used to define HTTP routes for the service.
   
