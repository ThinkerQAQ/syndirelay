export const PLATFORM_AUTH = Object.freeze([
  {
    id: "cnblogs",
    label: "博客园",
    probe: {
      kind: "json",
      url: "https://i.cnblogs.com/api/user",
      path: "loginName",
    },
  },
  {
    id: "juejin",
    label: "掘金",
    probe: {
      kind: "json",
      url: "https://api.juejin.cn/user_api/v1/user/get",
      path: "data.user_id",
    },
  },
  {
    id: "csdn",
    label: "CSDN",
    probe: {
      kind: "json",
      url: "https://g-api.csdn.net/community/toolbar-api/v1/get-user-info",
      path: "data.nickName",
    },
  },
  {
    id: "segmentfault",
    label: "思否",
    probe: {
      kind: "html",
      url: "https://segmentfault.com/user/settings",
      match: "href=[\"']\/u\/[^\"']+[\"']",
    },
  },
  {
    id: "zhihu",
    label: "知乎",
    probe: {
      kind: "json",
      url: "https://www.zhihu.com/api/v4/me",
      path: "id",
      headers: { "x-requested-with": "fetch" },
    },
  },
  {
    id: "51cto",
    label: "51CTO",
    probe: {
      kind: "html",
      url: "https://blog.51cto.com/blogger/publish",
      match: "https?://blog\\.51cto\\.com/[A-Za-z0-9_-]+/?[\"']",
    },
  },
  {
    id: "oschina",
    label: "开源中国",
    probe: {
      kind: "json",
      url: "https://apiv1.oschina.net/oschinapi/user/myDetails",
      path: "result.userId",
    },
  },
  {
    id: "toutiao",
    label: "今日头条",
    probe: {
      kind: "json",
      url: "https://mp.toutiao.com/mp/agw/media/user_login_status_api",
      path: "data.is_login",
      equals: true,
    },
  },
  {
    id: "devto",
    label: "DEV.to",
    probe: {
      kind: "final-url",
      url: "https://dev.to/settings/account",
      loggedOutPathPatterns: ["/enter", "/login", "/users/sign_in"],
    },
  },
  {
    id: "hashnode",
    label: "Hashnode",
    probe: { kind: "cookies", cookieUrl: "https://hashnode.com/", requiredCookieNames: [], requireAny: true },
  },
  {
    id: "medium",
    label: "Medium",
    probe: {
      kind: "cookies",
      cookieUrl: "https://medium.com/",
      requiredCookieNames: ["sid"],
    },
  },
]);

export const PLATFORM_SESSIONS = Object.freeze({
  cnblogs: {
    sessionProbeUrl: "https://i.cnblogs.com/api/user",
    cookieDomains: ["cnblogs.com"],
    // Chrome cookies.getAll defaults to unpartitioned cookies. CNBlogs can also
    // have cookies scoped to its top-level site partition.
    cookiePartitionKeys: [{ topLevelSite: "https://cnblogs.com" }],
    cookieUrls: [
      "https://www.cnblogs.com/",
      "https://i.cnblogs.com/",
      "https://i.cnblogs.com/api/user",
      "https://i.cnblogs.com/api/posts",
      "https://i.cnblogs.com/posts/edit",
      "https://home.cnblogs.com/",
      "https://account.cnblogs.com/",
      "https://upload.cnblogs.com/v2/images/cors-upload",
    ],
    requiredCookieNames: [],
  },
  juejin: {
    sessionProbeUrl: "https://api.juejin.cn/user_api/v1/user/get",
    cookieDomains: ["juejin.cn"],
    cookieUrls: ["https://juejin.cn/", "https://api.juejin.cn/"],
    requiredCookieNames: [],
  },
  csdn: {
    sessionProbeUrl: "https://bizapi.csdn.net/blog-console-api/v3/editor/getBaseInfo",
    cookieDomains: ["csdn.net"],
    cookieUrls: [
      "https://www.csdn.net/",
      "https://blog.csdn.net/",
      "https://editor.csdn.net/",
      "https://bizapi.csdn.net/",
      "https://g-api.csdn.net/",
    ],
    cookieNames: [
      "UserName", "UserToken", "UserInfo", "UserNick",
      "AU", "UN", "BT", "csrfToken", "SESSION",
    ],
    requiredCookieNames: [],
  },
  segmentfault: {
    sessionProbeUrl: "https://segmentfault.com/write",
    cookieDomains: ["segmentfault.com"],
    cookieUrls: ["https://segmentfault.com/", "https://segmentfault.com/user/settings/profile", "https://segmentfault.com/write"],
    cookieNames: ["PHPSESSID", "SHARESESSID", "sl-session", "_c_WBKFRo"],
    requiredCookieNames: [],
  },
  zhihu: {
    sessionProbeUrl: "https://www.zhihu.com/api/v4/me",
    cookieDomains: ["zhihu.com"],
    cookieUrls: ["https://www.zhihu.com/", "https://www.zhihu.com/api/v4/me", "https://zhuanlan.zhihu.com/"],
    cookieNames: ["z_c0", "_xsrf", "d_c0", "__zse_ck", "SESSIONID", "BEC"],
    requiredCookieNames: [],
  },
  "51cto": {
    sessionProbeUrl: "https://blog.51cto.com/blogger/publish",
    cookieDomains: ["51cto.com"],
    cookieUrls: [
      "https://www.51cto.com/",
      "https://blog.51cto.com/",
      "https://api-media.51cto.com/",
      "https://api-blog.51cto.com/",
      "https://ucenter.51cto.com/",
    ],
    cookieNames: [
      "www51cto", "pub_auth_profile", "pub_sauth1", "pub_sauth2",
      "pub_cookietime", "pub_wechatopen", "once_p", "PHPSESSID",
      "EO-Bot-Captcha-Token", "EO-Bot-Js-Token",
    ],
    requiredCookieNames: [],
  },
  oschina: {
    sessionProbeUrl: "https://apiv1.oschina.net/oschinapi/user/myDetails",
    cookieDomains: ["oschina.net"],
    cookieUrls: ["https://www.oschina.net/", "https://my.oschina.net/", "https://apiv1.oschina.net/"],
    cookieNames: ["oscid", "_user_behavior_", "sl-session", "BEC"],
    requiredCookieNames: [],
  },
  toutiao: {
    sessionProbeUrl: "https://mp.toutiao.com/mp/agw/media/user_login_status_api",
    cookieDomains: ["toutiao.com"],
    cookieUrls: ["https://mp.toutiao.com/"],
    requiredCookieNames: [],
  },
  devto: {
    sessionProbeUrl: "https://dev.to/dashboard",
    cookieDomains: ["dev.to"],
    cookieUrls: ["https://dev.to/", "https://dev.to/dashboard", "https://dev.to/new"],
    cookieNames: ["_Devto_Forem_Session", "remember_user_token", "current_user"],
    requiredCookieNames: [],
    optional: true,
  },
  hashnode: {
    sessionProbeUrl: "https://hashnode.com/api/publications",
    cookieDomains: ["hashnode.com"],
    cookieUrls: ["https://hashnode.com/", "https://hashnode.com/api/publications"],
    requiredCookieNames: [],
  },
  medium: {
    sessionProbeUrl: "https://medium.com/me/stories",
    cookieDomains: ["medium.com"],
    cookieUrls: ["https://medium.com/", "https://medium.com/me/stories", "https://medium.com/_/graphql"],
    cookiePartitionKeys: [{ topLevelSite: "https://medium.com" }],
    cookieNames: ["sid", "uid", "rid", "xsrf", "cf_clearance", "_cfuvid"],
    requiredCookieNames: [],
  },
});

export const PLATFORM_HOSTS = Object.freeze({
  cnblogs: "https://*.cnblogs.com/*",
  juejin: "https://*.juejin.cn/*",
  csdn: "https://*.csdn.net/*",
  segmentfault: "https://*.segmentfault.com/*",
  zhihu: "https://*.zhihu.com/*",
  "51cto": "https://*.51cto.com/*",
  oschina: "https://*.oschina.net/*",
  toutiao: "https://*.toutiao.com/*",
  devto: "https://dev.to/*",
  medium: "https://medium.com/*",
  hashnode: "https://hashnode.com/*",
});
