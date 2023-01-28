package repository

import (
	"FSchedule/domain/enum"
	"FSchedule/domain/taskGroup"
	"FSchedule/infrastructure/repository/model"
	"github.com/farseer-go/cache"
	"github.com/farseer-go/collections"
	"github.com/farseer-go/data"
	"github.com/farseer-go/fs/container"
	"github.com/farseer-go/fs/dateTime"
	"github.com/farseer-go/mapper"
	"github.com/farseer-go/redis"
	"time"
)

type taskGroupRepository struct {
	TaskGroup   data.TableSet[model.TaskGroupPO] `data:"name=task_group"`
	Task        data.TableSet[model.TaskPO]      `data:"name=task"`
	redis       *redis.Client
	CacheManage cache.ICacheManage[taskGroup.DomainObject] `inject:"FSS_TaskGroup"`
}

func registerTaskGroupRepository() {
	repository := data.NewContext[taskGroupRepository]("default")
	repository.redis = redis.NewClient("default")
	cacheManage := redis.SetProfiles[taskGroup.DomainObject]("FSS_TaskGroup", "Name", 0, "default")

	// 多级缓存
	cacheManage.SetListSource(func() collections.List[taskGroup.DomainObject] {
		var lst collections.List[taskGroup.DomainObject]
		repository.TaskGroup.ToList().MapToList(&lst)
		return lst
	})

	cacheManage.SetItemSource(func(cacheId any) (taskGroup.DomainObject, bool) {
		po := repository.TaskGroup.Where("Name = ?", cacheId).ToEntity()
		if po.Name != "" {
			return mapper.Single[taskGroup.DomainObject](&po), true
		}
		var do taskGroup.DomainObject
		return do, false
	})

	// 注册仓储
	container.RegisterInstance[taskGroup.Repository](repository)
}

func (repository taskGroupRepository) ToList() collections.List[taskGroup.DomainObject] {
	return repository.CacheManage.Get()
}

func (repository taskGroupRepository) ToEntity(name string) taskGroup.DomainObject {
	item, _ := repository.CacheManage.GetItem(name)
	return item
}

func (repository taskGroupRepository) TodayFailCount() int64 {
	return repository.Task.Where("status = ? and create_at >= ?", enum.Fail, dateTime.Now().Date().ToTime()).Count()
}

func (repository taskGroupRepository) ToTaskSpeedList(name string) []int64 {
	lstPO := repository.Task.Where("name = ? and status = ?", name, enum.Success).Desc("create_at").Select("RunSpeed").Limit(100).ToList()
	var lstSpeed []int64
	lstPO.Select(&lstSpeed, func(item model.TaskPO) any {
		return item.RunSpeed
	})
	return lstSpeed
}

func (repository taskGroupRepository) ToListByClientId(clientId int64) collections.List[taskGroup.DomainObject] {
	lst := repository.ToList()
	return lst.Where(func(item taskGroup.DomainObject) bool {
		return item.Task.Client.Id == clientId && item.Task.StartAt.UnixMicro() < time.Now().UnixMicro()
	}).ToList()
}

func (repository taskGroupRepository) GetTaskGroupCount() int64 {
	return int64(repository.CacheManage.Count())
}

func (repository taskGroupRepository) AddTask(taskDO taskGroup.TaskEO) {
	po := mapper.Single[model.TaskPO](&taskDO)
	repository.Task.Insert(&po)
}

func (repository taskGroupRepository) Add(do *taskGroup.DomainObject) {
	po := mapper.Single[model.TaskGroupPO](do)
	po.ActivateAt = time.Now()
	po.LastRunAt = time.Now()
	po.NextAt = time.Now()
	repository.TaskGroup.Insert(&po)
	repository.CacheManage.SaveItem(*do)
}

func (repository taskGroupRepository) Save(do taskGroup.DomainObject) {
	do.NeedSave = false
	repository.CacheManage.SaveItem(do)
}

func (repository taskGroupRepository) Delete(name string) {
	repository.Task.Where("name = ?", name).Delete()
	repository.TaskGroup.Where("name = ?", name).Delete()
	repository.CacheManage.Remove(name)
}

func (repository taskGroupRepository) ToUnRunCount() int {
	return repository.ToList().Where(func(item taskGroup.DomainObject) bool {
		return item.Task.Status == enum.None || item.Task.Status == enum.Scheduler || item.Task.CreateAt.UnixMicro() < time.Now().UnixMicro()
	}).Count()
}

func (repository taskGroupRepository) ToSchedulerWorkingList() collections.List[taskGroup.DomainObject] {
	return repository.ToList().Where(func(item taskGroup.DomainObject) bool {
		return item.Task.Status == enum.Scheduler || item.Task.Status == enum.Working
	}).ToList()
}

func (repository taskGroupRepository) GetTaskUnFinishList(jobsNames []string, top int) collections.List[taskGroup.DomainObject] {
	return repository.ToList().Where(func(item taskGroup.DomainObject) bool {
		return item.IsEnable && collections.NewList(jobsNames...).Contains(item.Name) && item.Task.Status != enum.Success && item.Task.Status != enum.Fail
	}).OrderBy(func(item taskGroup.DomainObject) any {
		return item.NextAt.UnixMicro()
	}).Take(top).ToList()
}

// ClearFinish 清除成功的任务记录（1天前）
func (repository taskGroupRepository) ClearFinish(name string, taskId int) {
	repository.Task.Where("name = ? and (status = ? or status = ?) and create_at < ? and Id < ?", name, enum.Success, enum.Fail, time.Now().Add(-24*time.Hour), taskId).Delete()
}

// SaveToDb 保存到数据库
func (repository taskGroupRepository) SaveToDb(do taskGroup.DomainObject) {
	po := mapper.Single[model.TaskGroupPO](&do)
	repository.TaskGroup.Where("name = ?", do.Name).Update(po)
}

// ToIdList 从数据库中读取数据
func (repository taskGroupRepository) ToIdList() []string {
	lst := repository.TaskGroup.Select("name").ToList()
	var lstName []string
	lst.Select(&lstName, func(item model.TaskGroupPO) any {
		return item.Name
	})
	return lstName
}

func (repository taskGroupRepository) ToListByGroupId(name string, pageSize int, pageIndex int) collections.PageList[taskGroup.TaskEO] {
	page := repository.Task.Where("name = ?", name).Desc("create_at").ToPageList(pageSize, pageIndex)
	return repository.toPageListTaskEO(page)
}

//func (repository taskGroupRepository) GetCanSchedulerTaskGroup(jobsName []string, ts time.Duration, count int, client taskGroup.ClientVO) collections.List[taskGroup.TaskEO] {
//	getLocker := repository.redis.Lock.GetLocker("FSS_Scheduler", 5*time.Second)
//	if !getLocker.TryLock() {
//		flog.Warningf("调度任务时加锁失败，Job=%s，ClientIp=%s", collections.NewList(jobsName...).ToString(","), client.Ip)
//		return collections.NewList[taskGroup.TaskEO]()
//	}
//	defer getLocker.ReleaseLock()
//	lstSchedulerTaskGroup := repository.ToList().Where(func(item taskGroup.DomainObject) bool {
//		return item.CanScheduler(jobsName, ts)
//	}).OrderBy(func(item taskGroup.DomainObject) any {
//		return item.StartAt.UnixMicro()
//	}).Take(count)
//
//	lst := collections.NewList[taskGroup.TaskEO]()
//	for _, taskGroupDO := range lstSchedulerTaskGroup.ToArray() {
//		// 设为调度状态
//		taskGroupDO.Scheduler(client)
//		repository.Save(taskGroupDO)
//		// 如果不相等，说明被其它客户端拿了
//		lst.Add(taskGroupDO.Task)
//		if taskGroupDO.Task.TaskGroupId == 0 {
//			flog.Errorf("发现taskGroupDO.Task.TaskGroupId=0的数据，val=%v", taskGroupDO.Task)
//		}
//	}
//	return lst
//}

func (repository taskGroupRepository) GetEnableTaskList(status enum.TaskStatus, pageSize int, pageIndex int) collections.PageList[taskGroup.TaskEO] {
	lstTaskGroup := repository.ToList().Where(func(item taskGroup.DomainObject) bool {
		return item.IsEnable
	}).ToList()

	if status != enum.None {
		lstTaskGroup = lstTaskGroup.Where(func(item taskGroup.DomainObject) bool {
			return item.Task.Status == status
		}).ToList()
	}

	lstTaskGroup = lstTaskGroup.OrderBy(func(item taskGroup.DomainObject) any {
		return item.Name
	}).ToList()

	var lst collections.List[taskGroup.TaskEO]
	lstTaskGroup.Select(&lst, func(item taskGroup.DomainObject) any {
		return item.Task
	})
	return lst.ToPageList(pageSize, pageIndex)
}

func (repository taskGroupRepository) ToFinishPageList(pageSize int, pageIndex int) collections.PageList[taskGroup.TaskEO] {
	page := repository.Task.Where("(status = ? or status = ?) and (create_at >= ?)", enum.Fail, enum.Success, time.Now().Add(-24*time.Hour)).
		Desc("run_at").ToPageList(pageSize, pageIndex)
	return repository.toPageListTaskEO(page)
}

func (repository taskGroupRepository) ToFinishList(name string, top int) collections.List[taskGroup.TaskEO] {
	lstPO := repository.Task.Where("name = ? and (status = ? or status = ?)", name, enum.Success, enum.Fail).Desc("create_at").Limit(top).ToList()
	return mapper.ToList[taskGroup.TaskEO](lstPO)
}

func (repository taskGroupRepository) toPageListTaskEO(page collections.PageList[model.TaskPO]) collections.PageList[taskGroup.TaskEO] {
	lst := mapper.ToList[taskGroup.TaskEO](page.List)
	return collections.NewPageList[taskGroup.TaskEO](lst, page.RecordCount)
}

func (repository taskGroupRepository) toListTaskEO(lstPO collections.List[model.TaskPO]) collections.List[taskGroup.TaskEO] {
	var lst collections.List[taskGroup.TaskEO]
	lstPO.Select(&lst, func(item model.TaskPO) any {
		eo := mapper.Single[taskGroup.TaskEO](&item)
		eo.Client.Id = item.ClientId
		eo.Client.Ip = item.ClientIp
		eo.Client.Name = item.ClientName
		return eo
	})
	return lst
}
