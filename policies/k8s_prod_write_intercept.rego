package ballast.security

# Ballast K8s 生产写操作拦截策略（spec/INIT.md 第 466-517 行原样）
#
# 默认状态：所有行为拒绝，除非命中放行白名单
default allow = false
default action = "SUSPEND" # 触发安全策略时的默认首选行为：挂起进程进入人工审批断点

# 规则一：绝对允许放行名单 (只读和安全的 Git 操作)
allow {
	is_safe_command(input.command)
}

# 规则二：绝对阻断黑名单 (任何环境下 AI 引擎绝对不能碰的红线命令)
action = "DENY" {
	blacklist_commands[_] == input.command
}

# 规则三：触发人工审批断点的灰名单 (允许 AI 在沙箱内 Plan，但执行必须截获并悬挂)
action = "SUSPEND" {
	not is_safe_command(input.command)
	graylist_commands[_] == input.command
}

# --- 策略底座元数据声明 ---

is_safe_command(cmd) {
	safe_commands[_] == cmd
}

# 只读命令白名单 (Auto-Run)
safe_commands = [
	"kubectl get", "kubectl logs", "kubectl describe", "kubectl top",
	"git status", "git diff", "git log",
	"terraform plan", "terraform validate",
	"ls", "cat", "grep", "awk"
]

# 危险命令黑名单 (Blocked)
blacklist_commands = [
	"rm -rf /", "mkfs", "fdisk", "dd",
	"kubectl delete namespace", "kubectl delete clusterrolebinding",
	"shutdown", "reboot"
]

# 高危变更灰名单 (Human-in-the-loop Intercept)
graylist_commands = [
	"kubectl apply", "kubectl delete", "kubectl patch", "kubectl edit",
	"terraform apply", "terraform destroy",
	"git push", "helm upgrade", "helm uninstall"
]
