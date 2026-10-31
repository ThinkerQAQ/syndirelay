package publisher

import "net/http"

func newAdapter(platform string, base *http.Client, session Session) (Adapter, error) {
	switch platform {
	case "hashnode":
		return NewHashnodeAdapter(base, session)
	case "devto":
		return NewDEVToAdapter(base, session)
	case "juejin":
		return NewJuejinAdapter(base, session)
	case "segmentfault":
		return NewSegmentFaultAdapter(base, session)
	case "oschina":
		return NewOSChinaAdapter(base, session)
	case "cnblogs":
		return NewCNBlogsAdapter(base, session)
	case "csdn":
		return NewCSDNAdapter(base, session)
	case "51cto":
		return New51CTOAdapter(base, session)
	case "zhihu":
		return NewZhihuAdapter(base, session)
	case "toutiao":
		return NewToutiaoAdapter(base, session)
	default:
		return nil, platformError(ErrNotImplemented, platform, "adapter", 0, "native adapter is not implemented", false)
	}
}
