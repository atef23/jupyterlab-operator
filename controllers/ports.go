package controllers

import (
	v1 "k8s.io/api/core/v1"
)

var jupyterlabPort = port{"jupyterlab", 8888}

type port struct {
	name string
	port int32
}

type ports []port

func (port port) asServicePort() v1.ServicePort {
	return v1.ServicePort{Name: port.name, Port: port.port}
}

func (port port) asServicePorts() []v1.ServicePort {
	return []v1.ServicePort{port.asServicePort()}
}
