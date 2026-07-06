# skill: k8s-log-harvester

触发关键词：CrashLoopBackOff、Pod 异常、K8s 排障、捞日志

# K8s 日志捞取剧本（样例）

当 K8s Pod 进入 CrashLoopBackOff 或异常重启状态时：

1. 执行 `kubectl get pods -n <ns>` 定位异常 Pod
2. 执行 `kubectl logs <pod> -n <ns>` 捞取最近日志
3. 执行 `kubectl describe pod <pod> -n <ns>` 查看事件
4. 汇总根因证据链，准备自愈建议（写操作须经 Ballast Harness 审批）

只读命令自动放行；任何 `kubectl apply/delete/patch` 触发人工审批断点。
