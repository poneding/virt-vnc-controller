# virt-vnc-controller

virt-vnc-controller 监听 kubevirt VirtualMachine 资源的创建，自动生成虚拟机 virt-vnc 服务。

virt-vnc： [poneding/virt-vnc](https://github.com/poneding/virt-vnc)

## 部署

```bash
kubectl apply -f https://raw.githubusercontent.com/poneding/virt-vnc-controller/master/deploy/manifests.yaml
```

等待部署完成：

```bash
kubectl get pod -n virt-vnc -w
```

## 测试

Note: 需要集群可以正常创建 Kubevirt 虚拟机。

```bash
kubectl apply -f https://raw.githubusercontent.com/poneding/virt-vnc-controller/master/deploy/cirros-vm.yaml
```

观察是否能正常创建 virt-vnc 服务：

```bash
kubectl get pod -n test -w
```

现在可以访问 virt-vnc 服务来查看虚拟机的图形界面。

清理：

```bash
kubectl delete -f https://raw.githubusercontent.com/poneding/virt-vnc-controller/master/deploy/cirros-vm.yaml
```

虚拟机删除后，virt-vnc-controller 会自动回收相关资源。

## 卸载

```bash
kubectl delete -f https://raw.githubusercontent.com/poneding/virt-vnc-controller/master/deploy/manifests.yaml
```
