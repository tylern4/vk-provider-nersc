# Helm Deployment Cheatsheet

## Install VK Provider (Dev)
```bash
helm install vk-nersc ./chart -f chart/values-dev.yaml
```

## Install VK Provider (Prod)
```bash
helm install vk-nersc ./chart -f chart/values-production.yaml
```

## Create a Workload Credential Secret
```bash
kubectl create secret generic sfapi-client \
  --from-file=sf_api.json=./sf_api.json

kubectl create secret generic globus-client \
  --from-literal=client_id="$GLOBUS_CLIENT_ID" \
  --from-literal=client_secret="$GLOBUS_CLIENT_SECRET"
```

## Enable StatefulSet via Helm Values
```yaml
statefulset:
  enabled: true
  name: hpc-stateful
  replicas: 3
  account: m1234
  credentialSecretName: sfapi-client
  credentialSecretKey: sf_api.json
  scratchBase: "/pscratch/sd/a/alice/vk-provider-nersc"
  globusCredentialSecretName: globus-client
  globusCredentialSecretKey: ""
  globusStagingCollectionID: nersc-collection-id
  inputSource: "globus://source-collection-id/path/to/data"
  outputDest: "globus://destination-collection-id/path/to/output"
  stageOut: "true"
  nodeSelector: perlmutter-vk
  container:
    name: compute
    image: registry.example.com/compute:latest
    command: ["python"]
    args: ["compute.py"]
  volumeMounts:
    - name: data
      mountPath: /mnt/data
      readOnly: false
  volumes:
    - name: data
      claimName: hpc-data-pvc
```

## Upgrade
```bash
helm upgrade vk-nersc ./chart -f chart/values-production.yaml
```

## Uninstall
```bash
helm uninstall vk-nersc
```
