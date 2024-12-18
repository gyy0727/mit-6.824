package raft

type ApplyMsg struct {
	CommandValid bool //*true为log，false为snapshot
	//* 向application层提交日志
	Command      interface{} //*由CommandValid决定
	CommandIndex int         //*日志的序号
	CommandTerm  int         //*任期

	// 向application层安装快照
	Snapshot          []byte
	LastIncludedIndex int
	LastIncludedTerm  int
}

// 日志项
type LogEntry struct {
	Command interface{} //*日志内容
	Term    int         //*任期
}

// 当前角色
const ROLE_LEADER = "Leader"         //*领导
const ROLE_FOLLOWER = "Follower"     //*追随者
const ROLE_CANDIDATES = "Candidates" //*候选
