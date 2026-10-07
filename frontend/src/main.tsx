import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  ShieldCheck,
  LayoutDashboard,
  FolderGit2,
  ScanLine,
  Users,
  Settings,
  ScrollText,
  LogOut,
  ArrowUpRight,
  ChevronRight,
  UserRound,
  LockKeyhole,
  Eye,
  EyeOff,
  FileCode2,
} from "lucide-react";
import { api, write, APIError, type User } from "./api";
import { ErrorBox } from "./components";
import {
  Overview,
  Runs,
  RunDetail,
  Projects,
  UserAdmin,
  Events,
} from "./pages";
import { Integrations } from "./integrations";
import { BotBindings } from "./bot-bindings";
import { WorkflowPolicies } from "./workflow-policy";
import { WorkspaceContext } from "./workspace-context";
import { WorkspaceMetrics } from "./workspace-metrics";
import { WorkspaceQueue } from "./workspace-queue";
import { SystemSettings } from "./settings";
import "./style.css";
import "./responsive.css";
import "./sequence.css";
import "./workspace.css";
import "./workspace-responsive.css";
import "./login.css";

function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [visiblePassword, setVisiblePassword] = useState(false);
  return (
    <main className="login">
      <header className="login-header">
        <a className="login-brand" href="#/">
          <span>
            <ShieldCheck size={23} />
          </span>
          AIMergeBot
        </a>
        <span className="login-header-caption">团队代码审计平台</span>
      </header>
      <div className="login-layout">
        <section className="login-intro" aria-labelledby="login-intro-title">
          <span className="login-kicker">
            <i />
            CODE REVIEW, WITH EVIDENCE
          </span>
          <h1 id="login-intro-title">
            从代码变更，
            <br />
            到可信的<span>审计。</span>
          </h1>
          <p>
            连接 Git 变更、AI 调查与人工复核。
            <br />
            每一次审计，都有明确的版本和可追溯的证据。
          </p>
          <div className="login-workflow" aria-label="审计流程">
            <div>
              <FileCode2 size={18} />
              <strong>提交快照</strong>
              <small>锁定审计版本</small>
            </div>
            <div>
              <ScanLine size={18} />
              <strong>AI 调查</strong>
              <small>收集代码证据</small>
            </div>
            <div>
              <ShieldCheck size={18} />
              <strong>人工复核</strong>
              <small>保留复核依据</small>
            </div>
          </div>
        </section>
        <section className="login-form" aria-labelledby="login-title">
          <div className="form-wrap">
            <span className="login-form-caption">访问你的工作空间</span>
            <h2 id="login-title">登录工作台</h2>
            <p className="muted">欢迎回来，使用团队账号继续。</p>
            <form
              onSubmit={async (e) => {
                e.preventDefault();
                setVisiblePassword(false);
                setBusy(true);
                setError("");
                try {
                  onLogin(
                    await api<User>(
                      "/auth/login",
                      write("POST", { username, password }),
                    ),
                  );
                } catch (e) {
                  setError(
                    e instanceof APIError && e.status === 401
                      ? "账号或密码不正确，请重新输入。"
                      : e instanceof APIError && e.status === 429
                        ? "尝试次数过多，请稍后再试。"
                        : "登录暂时不可用，请稍后重试。",
                  );
                } finally {
                  setBusy(false);
                }
              }}
            >
              <label>
                账号
                <span className="login-input">
                  <UserRound size={17} aria-hidden="true" />
                  <input
                    placeholder="输入你的团队账号"
                    name="username"
                    disabled={busy}
                    autoCapitalize="none"
                    spellCheck={false}
                    aria-describedby={error ? "login-error" : undefined}
                    autoComplete="username"
                    required
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                  />
                </span>
              </label>
              <label>
                密码
                <span className="login-input">
                  <LockKeyhole size={17} aria-hidden="true" />
                  <input
                    placeholder="输入密码"
                    id="login-password"
                    name="password"
                    aria-label="密码"
                    disabled={busy}
                    autoCapitalize="none"
                    spellCheck={false}
                    aria-describedby={error ? "login-error" : undefined}
                    type={visiblePassword ? "text" : "password"}
                    autoComplete="current-password"
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                  <button
                    type="button"
                    className="password-toggle"
                    aria-label={visiblePassword ? "隐藏密码" : "显示密码"}
                    aria-controls="login-password"
                    aria-pressed={visiblePassword}
                    disabled={busy}
                    onClick={() => setVisiblePassword(!visiblePassword)}
                  >
                    {visiblePassword ? <EyeOff size={18} /> : <Eye size={18} />}
                  </button>
                </span>
              </label>
              {error && (
                <div id="login-error">
                  <ErrorBox error={error} />
                </div>
              )}
              <button disabled={busy} className="primary full">
                {busy ? "正在登录…" : "进入工作台"}
                <ArrowUpRight size={17} />
              </button>
            </form>
            <p className="footnote">账号不可用？请联系团队管理员。</p>
            <div className="login-security">
              <LockKeyhole size={14} />
              <span>仅限授权团队成员访问</span>
            </div>
          </div>
        </section>
      </div>
      <div className="login-footer">
        <span>AIMergeBot · 代码审计工作台</span>
        <span>让每次变更，有据可查。</span>
      </div>
    </main>
  );
}
function App() {
  const [user, setUser] = useState<User | null>(null),
    [loading, setLoading] = useState(true),
    [sessionError, setSessionError] = useState(""),
    [route, setRoute] = useState(location.hash.slice(1) || "/"),
    [logoutError, setLogoutError] = useState("");
  useEffect(() => {
    api<User>("/auth/me")
      .then(setUser)
      .catch((e) => {
        if (e.status !== 401) setSessionError(e.message);
      })
      .finally(() => setLoading(false));
    const hash = () => setRoute(location.hash.slice(1) || "/");
    const expired = () => setUser(null);
    window.addEventListener("hashchange", hash);
    window.addEventListener("session-expired", expired);
    return () => {
      window.removeEventListener("hashchange", hash);
      window.removeEventListener("session-expired", expired);
    };
  }, []);
  if (loading) return <div className="boot">正在连接工作台…</div>;
  if (!user)
    return (
      <>
        <ErrorBox error={sessionError} />
        <Login
          onLogin={(u) => {
            setUser(u);
            setSessionError("");
          }}
        />
      </>
    );
  const links = [
    { path: "/", label: "概览", icon: LayoutDashboard },
    { path: "/tasks", label: "待办中心", icon: ScanLine },
    { path: "/runs", label: "审计任务", icon: ScanLine },
    { path: "/findings", label: "发现中心", icon: FileCode2 },
    { path: "/usage", label: "用量与成本", icon: ScrollText },
    { path: "/quality", label: "质量反馈", icon: FileCode2 },
    { path: "/projects", label: "项目", icon: FolderGit2 },
    { path: "/bot-bindings", label: "我的机器人绑定", icon: UserRound },
    ...(user.role === "admin"
      ? [
          { path: "/policies", label: "审计策略", icon: Settings },
          { path: "/context-repositories", label: "关联仓库", icon: FolderGit2 },
          { path: "/integrations", label: "集成与通知", icon: Settings },
          { path: "/users", label: "团队成员", icon: Users },
          { path: "/settings", label: "系统设置", icon: Settings },
          { path: "/events", label: "操作日志", icon: ScrollText },
        ]
      : []),
  ];
  const detail = route.match(/^\/runs\/(\d+)(?:\?.*)?$/);
  const title =
    links.find((x) => x.path === route)?.label ||
    (detail ? "审计详情" : "页面");
  return (
    <div className="shell">
      <aside className="sidebar">
        <a href="#/" className="brand">
          <ShieldCheck /> AIMergeBot
        </a>
        <span className="workspace-label">审计工作空间</span>
        {["日常工作", "管理"].map(group => {
          const grouped = links.filter(link => group === "管理" ? ["/policies", "/context-repositories", "/integrations", "/users", "/settings", "/events"].includes(link.path) : !["/policies", "/context-repositories", "/integrations", "/users", "/settings", "/events"].includes(link.path));
          return grouped.length ? <nav key={group} aria-label={group}>
          <span className="navigation-group">{group}</span>
          {grouped.map(({ path, label, icon: Icon }) => (
            <a
              key={path}
              href={"#" + path}
              aria-current={
                route === path || (path === "/runs" && detail)
                  ? "page"
                  : undefined
              }
              className={
                route === path || (path === "/runs" && detail) ? "active" : ""
              }
            >
              <Icon size={18} />
              {label}
              <ChevronRight size={14} />
            </a>
          ))}
        </nav> : null;
        })}
        <div className="sidebar-bottom">
          <div className="avatar">{user.username[0].toUpperCase()}</div>
          <div>
            <strong>{user.username}</strong>
            <small>{user.role === "admin" ? "管理员" : "团队成员"}</small>
          </div>
          <button
            title="退出登录"
            className="icon-button"
            onClick={async () => {
              try {
                await api("/auth/logout", write("POST"));
                setUser(null);
              } catch (e) {
                setLogoutError((e as Error).message);
              }
            }}
          >
            <LogOut size={17} />
          </button>
        </div>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <span>
            AIMergeBot <span className="slash">/</span> {title}
          </span>
          <span className="topbar-tag">
            <span className="dot" /> 团队审计工作台
          </span>
        </header>
        <main className="content">
          <ErrorBox error={logoutError} />
          {detail ? (
            <RunDetail id={Number(detail[1])} />
          ) : route === "/" ? (
            <Overview />
          ) : route === "/tasks" || route === "/findings" ? (
            <WorkspaceQueue key={route} mode={route === "/tasks" ? "tasks" : "findings"} />
          ) : route === "/usage" || route === "/quality" ? (
            <WorkspaceMetrics key={route} mode={route === "/usage" ? "usage" : "quality"} />
          ) : route === "/runs" ? (
            <Runs />
          ) : route === "/projects" ? (
            <Projects admin={user.role === "admin"} userID={user.id} />
          ) : route === "/bot-bindings" ? (
            <BotBindings />
          ) : route === "/policies" && user.role === "admin" ? (
            <WorkflowPolicies />
          ) : route === "/context-repositories" && user.role === "admin" ? (
            <WorkspaceContext />
          ) : route === "/integrations" && user.role === "admin" ? (
            <Integrations />
          ) : route === "/users" && user.role === "admin" ? (
            <UserAdmin />
          ) : route === "/settings" && user.role === "admin" ? (
            <SystemSettings />
          ) : route === "/events" && user.role === "admin" ? (
            <Events />
          ) : (
            <div>
              页面不存在或没有权限。<a href="#/">返回概览</a>
            </div>
          )}
        </main>
        <footer>
          提交固定 · 证据可查 · 人工复核 <span>AIMergeBot</span>
        </footer>
      </div>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
