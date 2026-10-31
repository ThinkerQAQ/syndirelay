package platform

type Capabilities struct {
	BrowserSession  bool `json:"browserSession"`
	APIKey          bool `json:"apiKey"`
	DraftCreate     bool `json:"draftCreate"`
	DraftUpdate     bool `json:"draftUpdate"`
	ExplicitPublish bool `json:"explicitPublish"`
	PublishedUpdate bool `json:"publishedUpdate"`
	// PublishedDraftEdit means an already-published article can be reopened as
	// an editable draft and published again without creating a duplicate post.
	PublishedDraftEdit bool `json:"publishedDraftEdit"`
	RemoteList         bool `json:"remoteList"`
	BodyImages         bool `json:"bodyImages"`
	BodyImageRehost    bool `json:"bodyImageRehost"`
	CoverImage         bool `json:"coverImage"`
	NativeCanonical    bool `json:"nativeCanonical"`
	Tags               bool `json:"tags"`
}

type Definition struct {
	ID              string
	Label           string
	DefaultLanguage string
	Capabilities    Capabilities
}

var definitions = []Definition{
	{
		ID: "cnblogs", Label: "博客园", DefaultLanguage: "zh-CN",
		Capabilities: Capabilities{
			BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true,
			PublishedUpdate: true, RemoteList: true, BodyImages: true, BodyImageRehost: true,
		},
	},
	{
		ID: "juejin", Label: "掘金", DefaultLanguage: "zh-CN",
		Capabilities: Capabilities{BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true, PublishedDraftEdit: true, RemoteList: true, BodyImages: true, BodyImageRehost: true},
	},
	{
		ID: "csdn", Label: "CSDN", DefaultLanguage: "zh-CN",
		Capabilities: Capabilities{BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true, PublishedDraftEdit: true, RemoteList: true, BodyImages: true, BodyImageRehost: true},
	},
	{
		ID: "segmentfault", Label: "思否", DefaultLanguage: "zh-CN",
		Capabilities: Capabilities{BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true, RemoteList: true, BodyImages: true, BodyImageRehost: true, Tags: true},
	},
	{
		ID: "zhihu", Label: "知乎", DefaultLanguage: "zh-CN",
		Capabilities: Capabilities{BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true, PublishedDraftEdit: true, RemoteList: true, BodyImages: true, BodyImageRehost: true},
	},
	{
		ID: "51cto", Label: "51CTO", DefaultLanguage: "zh-CN",
		Capabilities: Capabilities{BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true, RemoteList: true, BodyImages: true, BodyImageRehost: true, Tags: true},
	},
	{
		ID: "oschina", Label: "开源中国", DefaultLanguage: "zh-CN",
		// Current PublishDraft uses the ADD-article endpoint rather than
		// transitioning a draft ID. Until a draft-specific publish contract
		// is verified, never expose direct/create/update publish actions.
		Capabilities: Capabilities{BrowserSession: true, DraftCreate: true, DraftUpdate: true, PublishedDraftEdit: true, RemoteList: true, BodyImages: true, BodyImageRehost: true},
	},
	{
		ID: "toutiao", Label: "今日头条", DefaultLanguage: "zh-CN",
		// New publications require an explicit draft publish action.
		Capabilities: Capabilities{
			BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true,
			PublishedUpdate: true, RemoteList: true, BodyImages: true, BodyImageRehost: true,
		},
	},
	{
		ID: "devto", Label: "DEV.to", DefaultLanguage: "en",
		Capabilities: Capabilities{
			BrowserSession: true, APIKey: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true,
			PublishedUpdate: true, RemoteList: true, BodyImages: true, BodyImageRehost: true,
			CoverImage: true, NativeCanonical: true, Tags: true,
		},
	},
	{
		ID: "hashnode", Label: "Hashnode", DefaultLanguage: "en",
		Capabilities: Capabilities{
			BrowserSession: true, DraftCreate: true, DraftUpdate: true,
			RemoteList: true, // Read-only draft and published inventory.
			BodyImages: true, BodyImageRehost: true,
			CoverImage: true, NativeCanonical: true,
		},
	},
	{
		ID: "medium", Label: "Medium", DefaultLanguage: "en",
		Capabilities: Capabilities{
			BrowserSession: true, DraftCreate: true, DraftUpdate: true, ExplicitPublish: true,
			PublishedDraftEdit: true, RemoteList: true, BodyImages: true, BodyImageRehost: true,
		},
	},
}

var registry = func() map[string]Definition {
	result := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		result[definition.ID] = definition
	}
	return result
}()

func DefinitionFor(id string) (Definition, bool) {
	definition, ok := registry[id]
	return definition, ok
}

func For(id string) Capabilities {
	definition, _ := DefinitionFor(id)
	return definition.Capabilities
}

func Supported(id string) bool {
	_, ok := registry[id]
	return ok
}

func Label(id string) string {
	definition, _ := DefinitionFor(id)
	return definition.Label
}

func DefaultLanguage(id string) string {
	definition, _ := DefinitionFor(id)
	return definition.DefaultLanguage
}

func IDs() []string {
	result := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, definition.ID)
	}
	return result
}
