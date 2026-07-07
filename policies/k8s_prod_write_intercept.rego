package ballast.security

# 未知命令和所有生产写操作默认进入人工审批断点。
default decision = "SUSPEND"

# 复合 shell、解释器包装和提权命令可能隐藏第二条指令，绝不允许审批绕过。
decision = "DENY" {
	input.unsafe
}

decision = "DENY" {
	denied_wrappers[input.command]
}

# 明确的只读操作自动放行。command 是可执行文件，args 是参数数组，
# 避免把 "kubectl get" 与 "kubectl" 两种粒度混在一起。
decision = "APPROVE" {
	not input.unsafe
	input.command == "kubectl"
	safe_kubectl_verbs[input.args[0]]
}

decision = "APPROVE" {
	not input.unsafe
	input.command == "git"
	safe_git_verbs[input.args[0]]
}

decision = "APPROVE" {
	not input.unsafe
	input.command == "terraform"
	safe_terraform_verbs[input.args[0]]
}

decision = "APPROVE" {
	not input.unsafe
	safe_standalone_commands[input.command]
}

# 任何环境下都不可放行的破坏性命令。
decision = "DENY" {
	denied_commands[input.command]
}

decision = "DENY" {
	input.command == "rm"
	input.args[_] == "/"
}

decision = "DENY" {
	input.command == "kubectl"
	input.args[0] == "delete"
	denied_kubectl_resources[input.args[_]]
}

safe_kubectl_verbs = {"get", "logs", "describe", "top", "wait"}
safe_git_verbs = {"status", "diff", "log"}
safe_terraform_verbs = {"plan", "validate"}
safe_standalone_commands = {"ls", "cat", "grep", "awk"}

denied_commands = {"mkfs", "fdisk", "dd", "shutdown", "reboot"}
denied_kubectl_resources = {"namespace", "clusterrolebinding"}
denied_wrappers = {"sh", "bash", "zsh", "sudo", "env", "eval"}
