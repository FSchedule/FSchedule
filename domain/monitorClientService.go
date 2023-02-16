package domain

import (
	"FSchedule/domain/client"
	"context"
	"github.com/farseer-go/collections"
	"github.com/farseer-go/fs"
	"github.com/farseer-go/fs/container"
	"github.com/farseer-go/fs/core"
	"github.com/farseer-go/fs/flog"
	"time"
)

// 客户端列表
var clientList = collections.NewDictionary[int64, *ClientMonitor]()

// ClientMonitor 等待任务执行
type ClientMonitor struct {
	client           *client.DomainObject
	ctx              context.Context
	cancelFunc       context.CancelFunc
	ClientRepository client.Repository
	ClientJoinEvent  core.IEvent `inject:"ClientJoin"`
}

// MonitorClientPush 将最新的客户端信息，推送到监控线程
func MonitorClientPush(clientDO *client.DomainObject) {
	needPushUpdate := false
	//flog.Debugf("客户端（%d）更新通知：%s:%d", clientDO.Id, clientDO.Ip, clientDO.Port)
	// 新客户端
	if !clientDO.IsOffline() && !clientList.ContainsKey(clientDO.Id) {
		needPushUpdate = true
		ctx, cancelFunc := context.WithCancel(fs.Context)
		clientMonitor := container.ResolveIns(&ClientMonitor{
			client:     clientDO,
			ctx:        ctx,
			cancelFunc: cancelFunc,
		})
		clientList.Add(clientDO.Id, clientMonitor)

		flog.Infof("客户端（%d）开始监听：%s:%d", clientDO.Id, clientDO.Ip, clientDO.Port)
		// 异步检查客户端在线状态
		go clientMonitor.checkOnline()
	}

	existsClientDO := clientList.GetValue(clientDO.Id)

	if existsClientDO != nil {
		// 非新客户端时，对比前后状态是否不一致，有变化时，才需要通知任务组
		if !needPushUpdate {
			needPushUpdate = existsClientDO.client.Status != clientDO.Status
		}

		// 修改地址对应的值
		*existsClientDO.client = *clientDO
		if needPushUpdate {
			ClientUpdate(existsClientDO.client)
		}

		// 客户端离线
		if clientDO.IsOffline() {
			existsClientDO.cancelFunc()
			clientList.Remove(clientDO.Id)
		}
	}
}

// checkOnline 异步检查客户端在线状态
func (receiver *ClientMonitor) checkOnline() {
	for {
		if receiver.client.IsOffline() {
			return
		}
		select {
		case <-time.After(30 * time.Second):
			if !receiver.client.IsOffline() {
				receiver.client.CheckOnline()
				receiver.ClientRepository.Save(receiver.client)
			}
		case <-receiver.ctx.Done():
			return
		}
	}
}

// ClientCount 返回当前正在监控的客户端数量
func ClientCount() int {
	return clientList.Count()
}
