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
1. Define custom annotations for service object (K8s service object). These annotations shall allow users to define TCP/UPD or HTTP routes
2. Create a custom controller which will..
   - Watch for k8s services being create.modified or deleted.
   - Read custom annotation from these services and update TCP Configmap, UDP configmap and HTTP/s ingress whichever is required.
   - Update port mapping information in Nginx controller service object.
   
![Custom controller](/pictures/Picture4.png)   
   
