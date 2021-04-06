// Copyright 2020 Kentaro Hibino. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

// Package asynqtest defines test helpers for asynq and its internal packages.
package asynqtest

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/hibiken/asynq/internal/base"
)

type taskState int

const (
	stateActive taskState = iota
	statePending
	stateScheduled
	stateRetry
	stateArchived
)

var taskStateNames = map[taskState]string{
	stateActive:    "active",
	statePending:   "pending",
	stateScheduled: "scheduled",
	stateRetry:     "retry",
	stateArchived:  "archived",
}

func (s taskState) String() string {
	return taskStateNames[s]
}

// EquateInt64Approx returns a Comparer option that treats int64 values
// to be equal if they are within the given margin.
func EquateInt64Approx(margin int64) cmp.Option {
	return cmp.Comparer(func(a, b int64) bool {
		return math.Abs(float64(a-b)) <= float64(margin)
	})
}

// SortMsgOpt is a cmp.Option to sort base.TaskMessage for comparing slice of task messages.
var SortMsgOpt = cmp.Transformer("SortTaskMessages", func(in []*base.TaskMessage) []*base.TaskMessage {
	out := append([]*base.TaskMessage(nil), in...) // Copy input to avoid mutating it
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
})

// SortZSetEntryOpt is an cmp.Option to sort ZSetEntry for comparing slice of zset entries.
var SortZSetEntryOpt = cmp.Transformer("SortZSetEntries", func(in []base.Z) []base.Z {
	out := append([]base.Z(nil), in...) // Copy input to avoid mutating it
	sort.Slice(out, func(i, j int) bool {
		return out[i].Message.ID.String() < out[j].Message.ID.String()
	})
	return out
})

// SortTaskInfos is an cmp.Option to sort TaskInfo for comparing slice of task infos.
var SortTaskInfos = cmp.Transformer("SortTaskInfos", func(in []*base.TaskInfo) []*base.TaskInfo {
	out := append([]*base.TaskInfo(nil), in...) // Copy input to avoid mutating it
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
})

// SortServerInfoOpt is a cmp.Option to sort base.ServerInfo for comparing slice of process info.
var SortServerInfoOpt = cmp.Transformer("SortServerInfo", func(in []*base.ServerInfo) []*base.ServerInfo {
	out := append([]*base.ServerInfo(nil), in...) // Copy input to avoid mutating it
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		return out[i].PID < out[j].PID
	})
	return out
})

// SortWorkerInfoOpt is a cmp.Option to sort base.WorkerInfo for comparing slice of worker info.
var SortWorkerInfoOpt = cmp.Transformer("SortWorkerInfo", func(in []*base.WorkerInfo) []*base.WorkerInfo {
	out := append([]*base.WorkerInfo(nil), in...) // Copy input to avoid mutating it
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
})

// SortSchedulerEntryOpt is a cmp.Option to sort base.SchedulerEntry for comparing slice of entries.
var SortSchedulerEntryOpt = cmp.Transformer("SortSchedulerEntry", func(in []*base.SchedulerEntry) []*base.SchedulerEntry {
	out := append([]*base.SchedulerEntry(nil), in...) // Copy input to avoid mutating it
	sort.Slice(out, func(i, j int) bool {
		return out[i].Spec < out[j].Spec
	})
	return out
})

// SortSchedulerEnqueueEventOpt is a cmp.Option to sort base.SchedulerEnqueueEvent for comparing slice of events.
var SortSchedulerEnqueueEventOpt = cmp.Transformer("SortSchedulerEnqueueEvent", func(in []*base.SchedulerEnqueueEvent) []*base.SchedulerEnqueueEvent {
	out := append([]*base.SchedulerEnqueueEvent(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].EnqueuedAt.Unix() < out[j].EnqueuedAt.Unix()
	})
	return out
})

// SortStringSliceOpt is a cmp.Option to sort string slice.
var SortStringSliceOpt = cmp.Transformer("SortStringSlice", func(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
})

// IgnoreIDOpt is an cmp.Option to ignore ID field in task messages when comparing.
var IgnoreIDOpt = cmpopts.IgnoreFields(base.TaskMessage{}, "ID")

// NewTaskMessage returns a new instance of TaskMessage given a task type and payload.
func NewTaskMessage(taskType string, payload []byte) *base.TaskMessage {
	return NewTaskMessageWithQueue(taskType, payload, base.DefaultQueueName)
}

// NewTaskMessageWithQueue returns a new instance of TaskMessage given a
// task type, payload and queue name.
func NewTaskMessageWithQueue(taskType string, payload []byte, qname string) *base.TaskMessage {
	return &base.TaskMessage{
		ID:       uuid.New(),
		Type:     taskType,
		Queue:    qname,
		Retry:    25,
		Payload:  payload,
		Timeout:  1800, // default timeout of 30 mins
		Deadline: 0,    // no deadline
	}
}

// JSON serializes the given key-value pairs into stream of bytes in JSON.
func JSON(kv map[string]interface{}) []byte {
	b, err := json.Marshal(kv)
	if err != nil {
		panic(err)
	}
	return b
}

// TaskMessageAfterRetry returns an updated copy of t after retry.
// It increments retry count and sets the error message.
func TaskMessageAfterRetry(t base.TaskMessage, errMsg string) *base.TaskMessage {
	t.Retried = t.Retried + 1
	t.ErrorMsg = errMsg
	return &t
}

// TaskMessageWithError returns an updated copy of t with the given error message.
func TaskMessageWithError(t base.TaskMessage, errMsg string) *base.TaskMessage {
	t.ErrorMsg = errMsg
	return &t
}

// MustMarshal marshals given task message and returns a json string.
// Calling test will fail if marshaling errors out.
func MustMarshal(tb testing.TB, msg *base.TaskMessage) string {
	tb.Helper()
	data, err := base.EncodeMessage(msg)
	if err != nil {
		tb.Fatal(err)
	}
	return string(data)
}

// MustUnmarshal unmarshals given string into task message struct.
// Calling test will fail if unmarshaling errors out.
func MustUnmarshal(tb testing.TB, data string) *base.TaskMessage {
	tb.Helper()
	msg, err := base.DecodeMessage([]byte(data))
	if err != nil {
		tb.Fatal(err)
	}
	return msg
}

// FlushDB deletes all the keys of the currently selected DB.
func FlushDB(tb testing.TB, r redis.UniversalClient) {
	tb.Helper()
	switch r := r.(type) {
	case *redis.Client:
		if err := r.FlushDB().Err(); err != nil {
			tb.Fatal(err)
		}
	case *redis.ClusterClient:
		err := r.ForEachMaster(func(c *redis.Client) error {
			if err := c.FlushAll().Err(); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			tb.Fatal(err)
		}
	}
}

// SeedPendingQueue initializes the specified queue with the given messages.
func SeedPendingQueue(tb testing.TB, r redis.UniversalClient, msgs []*base.TaskMessage, qname string) {
	tb.Helper()
	r.SAdd(base.AllQueues, qname)
	seedRedisList(tb, r, qname, msgs, statePending)
}

// SeedActiveQueue initializes the active queue with the given messages.
func SeedActiveQueue(tb testing.TB, r redis.UniversalClient, msgs []*base.TaskMessage, qname string) {
	tb.Helper()
	r.SAdd(base.AllQueues, qname)
	seedRedisList(tb, r, qname, msgs, stateActive)
}

// SeedScheduledQueue initializes the scheduled queue with the given messages.
func SeedScheduledQueue(tb testing.TB, r redis.UniversalClient, entries []base.Z, qname string) {
	tb.Helper()
	r.SAdd(base.AllQueues, qname)
	seedRedisZSet(tb, r, qname, entries, stateScheduled)
}

// SeedRetryQueue initializes the retry queue with the given messages.
func SeedRetryQueue(tb testing.TB, r redis.UniversalClient, entries []base.Z, qname string) {
	tb.Helper()
	r.SAdd(base.AllQueues, qname)
	seedRedisZSet(tb, r, qname, entries, stateRetry)
}

// SeedArchivedQueue initializes the archived queue with the given messages.
func SeedArchivedQueue(tb testing.TB, r redis.UniversalClient, entries []base.Z, qname string) {
	tb.Helper()
	r.SAdd(base.AllQueues, qname)
	seedRedisZSet(tb, r, qname, entries, stateArchived)
}

// SeedDeadlines initializes the deadlines set with the given entries.
func SeedDeadlines(tb testing.TB, r redis.UniversalClient, entries []base.Z, qname string) {
	tb.Helper()
	r.SAdd(base.AllQueues, qname)
	seedRedisZSet(tb, r, qname, entries, stateActive)
}

// SeedAllPendingQueues initializes all of the specified queues with the given messages.
//
// pending maps a queue name to a list of messages.
func SeedAllPendingQueues(tb testing.TB, r redis.UniversalClient, pending map[string][]*base.TaskMessage) {
	tb.Helper()
	for q, msgs := range pending {
		SeedPendingQueue(tb, r, msgs, q)
	}
}

// SeedAllActiveQueues initializes all of the specified active queues with the given messages.
func SeedAllActiveQueues(tb testing.TB, r redis.UniversalClient, active map[string][]*base.TaskMessage) {
	tb.Helper()
	for q, msgs := range active {
		SeedActiveQueue(tb, r, msgs, q)
	}
}

// SeedAllScheduledQueues initializes all of the specified scheduled queues with the given entries.
func SeedAllScheduledQueues(tb testing.TB, r redis.UniversalClient, scheduled map[string][]base.Z) {
	tb.Helper()
	for q, entries := range scheduled {
		SeedScheduledQueue(tb, r, entries, q)
	}
}

// SeedAllRetryQueues initializes all of the specified retry queues with the given entries.
func SeedAllRetryQueues(tb testing.TB, r redis.UniversalClient, retry map[string][]base.Z) {
	tb.Helper()
	for q, entries := range retry {
		SeedRetryQueue(tb, r, entries, q)
	}
}

// SeedAllArchivedQueues initializes all of the specified archived queues with the given entries.
func SeedAllArchivedQueues(tb testing.TB, r redis.UniversalClient, archived map[string][]base.Z) {
	tb.Helper()
	for q, entries := range archived {
		SeedArchivedQueue(tb, r, entries, q)
	}
}

// SeedAllDeadlines initializes all of the deadlines with the given entries.
func SeedAllDeadlines(tb testing.TB, r redis.UniversalClient, deadlines map[string][]base.Z) {
	tb.Helper()
	for q, entries := range deadlines {
		SeedDeadlines(tb, r, entries, q)
	}
}

func seedRedisList(tb testing.TB, c redis.UniversalClient, qname string, msgs []*base.TaskMessage, state taskState) {
	tb.Helper()
	var key string
	switch state {
	case statePending:
		key = base.PendingKey(qname)
	case stateActive:
		key = base.ActiveKey(qname)
	default:
		tb.Fatalf("cannot seed redis LIST with task state %s", state)
	}
	for _, msg := range msgs {
		if msg.Queue != qname {
			tb.Fatalf("msg.Queue and queue name do not match! You are trying to seed queue %q with message %+v", qname, msg)
		}
		encoded := MustMarshal(tb, msg)
		if err := c.LPush(key, msg.ID.String()).Err(); err != nil {
			tb.Fatal(err)
		}
		key := base.TaskKey(msg.Queue, msg.ID.String())
		var processAt int64
		if state == statePending {
			processAt = time.Now().Unix()
		}
		if state == stateActive {
			processAt = 0
		}
		data := map[string]interface{}{
			"msg":        encoded,
			"timeout":    msg.Timeout,
			"deadline":   msg.Deadline,
			"state":      strings.ToUpper(state.String()),
			"process_at": processAt,
		}
		if err := c.HSet(key, data).Err(); err != nil {
			tb.Fatal(err)
		}
	}
}

func seedRedisZSet(tb testing.TB, c redis.UniversalClient, qname string, items []base.Z, state taskState) {
	tb.Helper()
	var key string
	switch state {
	case stateScheduled:
		key = base.ScheduledKey(qname)
	case stateRetry:
		key = base.RetryKey(qname)
	case stateArchived:
		key = base.ArchivedKey(qname)
	case stateActive:
		key = base.DeadlinesKey(qname)
	default:
		tb.Fatalf("cannot seed redis ZSET with task state %s", state)
	}
	for _, item := range items {
		msg := item.Message
		if msg.Queue != qname {
			tb.Fatalf("msg.Queue and queue name do not match! You are trying to seed queue %q with message %+v", qname, msg)
		}
		encoded := MustMarshal(tb, msg)
		z := &redis.Z{Member: msg.ID.String(), Score: float64(item.Score)}
		if err := c.ZAdd(key, z).Err(); err != nil {
			tb.Fatal(err)
		}
		key := base.TaskKey(msg.Queue, msg.ID.String())
		var (
			processAt    int64
			lastFailedAt int64
		)
		if state == stateScheduled {
			processAt = item.Score
		}
		if state == stateRetry {
			processAt = item.Score
			lastFailedAt = time.Now().Unix()
		}
		if state == stateArchived {
			lastFailedAt = item.Score
		}
		data := map[string]interface{}{
			"msg":            encoded,
			"timeout":        msg.Timeout,
			"deadline":       msg.Deadline,
			"state":          strings.ToUpper(state.String()),
			"process_at":     processAt,
			"last_failed_at": lastFailedAt,
		}
		if err := c.HSet(key, data).Err(); err != nil {
			tb.Fatal(err)
		}
	}
}

// GetPendingMessages returns all pending messages in the given queue.
func GetPendingMessages(tb testing.TB, r redis.UniversalClient, qname string) []*base.TaskMessage {
	tb.Helper()
	return getMessagesFromList(tb, r, qname, base.PendingKey)
}

// GetActiveMessages returns all active messages in the given queue.
func GetActiveMessages(tb testing.TB, r redis.UniversalClient, qname string) []*base.TaskMessage {
	tb.Helper()
	return getMessagesFromList(tb, r, qname, base.ActiveKey)
}

// GetScheduledMessages returns all scheduled task messages in the given queue.
func GetScheduledMessages(tb testing.TB, r redis.UniversalClient, qname string) []*base.TaskMessage {
	tb.Helper()
	return getMessagesFromZSet(tb, r, qname, base.ScheduledKey)
}

// GetRetryMessages returns all retry messages in the given queue.
func GetRetryMessages(tb testing.TB, r redis.UniversalClient, qname string) []*base.TaskMessage {
	tb.Helper()
	return getMessagesFromZSet(tb, r, qname, base.RetryKey)
}

// GetArchivedMessages returns all archived messages in the given queue.
func GetArchivedMessages(tb testing.TB, r redis.UniversalClient, qname string) []*base.TaskMessage {
	tb.Helper()
	return getMessagesFromZSet(tb, r, qname, base.ArchivedKey)
}

// GetScheduledEntries returns all scheduled messages and its score in the given queue.
func GetScheduledEntries(tb testing.TB, r redis.UniversalClient, qname string) []base.Z {
	tb.Helper()
	return getMessagesFromZSetWithScores(tb, r, qname, base.ScheduledKey)
}

// GetRetryEntries returns all retry messages and its score in the given queue.
func GetRetryEntries(tb testing.TB, r redis.UniversalClient, qname string) []base.Z {
	tb.Helper()
	return getMessagesFromZSetWithScores(tb, r, qname, base.RetryKey)
}

// GetArchivedEntries returns all archived messages and its score in the given queue.
func GetArchivedEntries(tb testing.TB, r redis.UniversalClient, qname string) []base.Z {
	tb.Helper()
	return getMessagesFromZSetWithScores(tb, r, qname, base.ArchivedKey)
}

// GetDeadlinesEntries returns all task messages and its score in the deadlines set for the given queue.
func GetDeadlinesEntries(tb testing.TB, r redis.UniversalClient, qname string) []base.Z {
	tb.Helper()
	return getMessagesFromZSetWithScores(tb, r, qname, base.DeadlinesKey)
}

// Retrieves all messages stored under `keyFn(qname)` key in redis list.
func getMessagesFromList(tb testing.TB, r redis.UniversalClient, qname string, keyFn func(qname string) string) []*base.TaskMessage {
	tb.Helper()
	ids := r.LRange(keyFn(qname), 0, -1).Val()
	var msgs []*base.TaskMessage
	for _, id := range ids {
		data := r.HGet(base.TaskKey(qname, id), "msg").Val()
		msgs = append(msgs, MustUnmarshal(tb, data))
	}
	return msgs
}

// Retrieves all messages stored under `keyFn(qname)` key in redis zset (sorted-set).
func getMessagesFromZSet(tb testing.TB, r redis.UniversalClient, qname string, keyFn func(qname string) string) []*base.TaskMessage {
	tb.Helper()
	ids := r.ZRange(keyFn(qname), 0, -1).Val()
	var msgs []*base.TaskMessage
	for _, id := range ids {
		msg := r.HGet(base.TaskKey(qname, id), "msg").Val()
		msgs = append(msgs, MustUnmarshal(tb, msg))
	}
	return msgs
}

// Retrieves all messages along with their scores stored under `keyFn(qname)` key in redis zset (sorted-set).
func getMessagesFromZSetWithScores(tb testing.TB, r redis.UniversalClient, qname string, keyFn func(qname string) string) []base.Z {
	tb.Helper()
	zs := r.ZRangeWithScores(keyFn(qname), 0, -1).Val()
	var res []base.Z
	for _, z := range zs {
		msg := r.HGet(base.TaskKey(qname, z.Member.(string)), "msg").Val()
		res = append(res, base.Z{Message: MustUnmarshal(tb, msg), Score: int64(z.Score)})
	}
	return res
}

// GetRetryTaskInfos returns all retry tasks' TaskInfo from the given queue.
func GetRetryTaskInfos(tb testing.TB, r redis.UniversalClient, qname string) []*base.TaskInfo {
	tb.Helper()
	return getTaskInfosFromZSet(tb, r, qname, base.RetryKey)
}

// GetArchivedTaskInfos returns all archived tasks' TaskInfo from the given queue.
func GetArchivedTaskInfos(tb testing.TB, r redis.UniversalClient, qname string) []*base.TaskInfo {
	tb.Helper()
	return getTaskInfosFromZSet(tb, r, qname, base.ArchivedKey)
}

func getTaskInfosFromZSet(tb testing.TB, r redis.UniversalClient, qname string,
	keyFn func(qname string) string) []*base.TaskInfo {
	tb.Helper()
	ids := r.ZRange(keyFn(qname), 0, -1).Val()
	var tasks []*base.TaskInfo
	for _, id := range ids {
		vals := r.HMGet(base.TaskKey(qname, id), "msg", "state", "process_at", "last_failed_at").Val()
		info, err := makeTaskInfo(vals)
		if err != nil {
			tb.Fatalf("could not make task info from values returned by HMGET: %v", err)
		}
		tasks = append(tasks, info)
	}
	return tasks
}

// makeTaskInfo takes values returned from HMGET(TASK_KEY, "msg", "state", "process_at", "last_failed_at")
// command and return a TaskInfo. It assumes that `vals` contains four values for each field.
func makeTaskInfo(vals []interface{}) (*base.TaskInfo, error) {
	if len(vals) != 4 {
		return nil, fmt.Errorf("asynq internal error: HMGET command returned %d elements", len(vals))
	}
	// Note: The "msg", "state" fields are non-nil;
	// whereas the "process_at", "last_failed_at" fields can be nil.
	encoded := vals[0]
	if encoded == nil {
		return nil, fmt.Errorf("asynq internal error: HMGET field 'msg' was nil")
	}
	msg, err := base.DecodeMessage([]byte(encoded.(string)))
	if err != nil {
		return nil, err
	}
	state := vals[1]
	if state == nil {
		return nil, fmt.Errorf("asynq internal error: HMGET field 'state' was nil")
	}
	processAt, err := parseIntOrDefault(vals[2], 0)
	if err != nil {
		return nil, err
	}
	lastFailedAt, err := parseIntOrDefault(vals[3], 0)
	if err != nil {
		return nil, err
	}
	return &base.TaskInfo{
		TaskMessage:   msg,
		State:         strings.ToLower(state.(string)),
		NextProcessAt: processAt,
		LastFailedAt:  lastFailedAt,
	}, nil
}

// Parses val as base10 64-bit integer if val contains a value.
// Uses default value if val is nil.
//
// Assumes val contains either string value or nil.
func parseIntOrDefault(val interface{}, defaultVal int64) (int64, error) {
	if val == nil {
		return defaultVal, nil
	}
	return strconv.ParseInt(val.(string), 10, 64)
}
