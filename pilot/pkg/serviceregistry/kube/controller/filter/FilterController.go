package filter

type FilterFunc func(obj interface{}) bool

type FilterController interface {
	Add(executor FilterFunc) FilterController
	Filter(data interface{}) bool
}

type Filter struct {
	dataC     chan interface{}
	matchedC  chan bool
	executors []FilterFunc
}

func NewFilter(excutors []FilterFunc) FilterController {
	return &Filter{
		matchedC:  make(chan bool),
		executors: excutors,
	}
}

func (p *Filter) Add(executor FilterFunc) FilterController {
	p.executors = append(p.executors, executor)

	return p
}

func (p *Filter) Filter(data interface{}) bool {
	p.dataC <- data
	for i := 0; i < len(p.executors); i++ {
		p.dataC, p.matchedC = run(p.dataC, p.executors[i])
	}

	return <-p.matchedC
}

func run(inC <-chan interface{}, f FilterFunc) (chan interface{}, chan bool) {
	outC := make(chan interface{})
	matched := make(chan bool)

	outC <- inC
	go func() {
		defer close(outC)
		matched <- f(inC)
	}()

	return outC, matched
}
