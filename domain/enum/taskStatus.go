package enum

type TaskStatus int

const (
	None         TaskStatus = iota //  未开始
	Scheduling                     //  调度中
	ScheduleFail                   //  调度失败
	Working                        //  执行中
	Fail                           //  失败
	Success                        //  完成
)

func (e TaskStatus) String() string {
	switch e {
	case None:
		return "None"
	case Scheduling:
		return "Scheduling"
	case ScheduleFail:
		return "ScheduleFail"
	case Working:
		return "Working"
	case Fail:
		return "Fail"
	case Success:
		return "Success"
	}
	return "None"
}
