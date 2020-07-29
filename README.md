# jupyterlab-operator

This operator deploys the AI/ML Jupyter notebook IDE Jupyterlab in Openshift

## Requirements
- Openshift cluster
- Openshift CLI
- Docker CLI

## Deployment
Log in to an Openshift cluster from the Openshift CLI and run the following commands:

    git clone https://github.com/atef23/jupyterlab-operator.git
    cd jupyterlab-operator
    oc new-project jupyterlab
    make deploy IMG=quay.io/aaziz/jupyterlab-operator:v1.0.0
    oc apply -f config/samples/jupyter_v1alpha1_jupyterlab.yaml

Navigate to "routes" and access Jupyterlab from the created route. The authentication token to log in can be found in the logs of the Jupyterlab pod.
