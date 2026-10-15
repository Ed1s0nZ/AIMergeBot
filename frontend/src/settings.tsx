import { ModelBudgetSettingsFields } from "./model-budget-settings";
import { AuditQuotaSettingsFields } from "./audit-quota-settings";
import { GitAuditSettingsFields } from "./git-audit-settings";
import { useState } from "react";
import { useSystemSettings } from "./use-system-settings";
import { SettingsRecovery } from "./settings-recovery";
import {
  Save,
  KeyRound,
  Bot,
  GitBranch,
  SlidersHorizontal,
  CheckCircle2,
  ChevronRight,
  Gauge,
} from "lucide-react";
import { ErrorBox, Empty } from "./components";
export function SystemSettings() {
  const [section, setSection] = useState("model");
  const editor = useSystemSettings();
  const { settings, trustedProxyDraft, setTrustedProxyDraft, setSettings,
    error, saved, busy, dirty, load, canEdit, canSave } = editor;
  if (!settings)
    return <><ErrorBox error={error} /><Empty>
      {busy ? "加载系统设置…" : "尚未读取到系统设置。"}
      <button disabled={busy} onClick={() => void load()}>重新读取设置</button>
    </Empty></>;
  const s = settings;
  const groups = [
    {
      id: "model",
      title: "模型服务",
      description: "模型与推理参数",
      icon: Bot,
    },
    {
      id: "integration",
      title: "GitLab 集成",
      description: "仓库与自动触发",
      icon: GitBranch,
    },
    {
      id: "quotas",
      title: "审计配额",
      description: "队列容量与执行次数",
      icon: Gauge,
    },
    {
      id: "execution",
      title: "执行策略",
      description: "调查预算与服务配置",
      icon: SlidersHorizontal,
    },
  ];
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">工作空间 / 配置</span>
          <h1>系统设置</h1>
          <p>配置模型与集成。保存后同步到项目目录的 config.yaml。</p>
        </div>
        <span className="config-tag">
          <KeyRound size={15} />
          管理员专属
        </span>
      </div>
      <ErrorBox error={error} />
      {s.restart_required && (
        <div className="coverage" role="status">
          服务配置待重启：监听地址、并发数或可信代理与当前运行配置不同。重启前继续使用原来的服务配置。
        </div>
      )}
      <SettingsRecovery {...editor} />
      {s.project_config_sync?.pending && (
        <div className="error" role="status">
          项目配置尚未同步到 config.yaml，服务会自动重试。数据库中的修改已保留。
        </div>
      )}
      {saved && (
        <div className="success" role="status">
          {saved}
        </div>
      )}
      <div className="settings-layout">
        <aside className="settings-rail">
          <span className="rail-caption">配置分组</span>
          <div className="settings-nav" aria-label="配置分组">
            {groups.map(({ id, title, description, icon: Icon }) => (
              <button
                key={id}
                type="button"
                className={section === id ? "selected" : ""}
                aria-pressed={section === id}
                onClick={() => setSection(id)}
              >
                <Icon size={19} />
                <span>
                  <strong>{title}</strong>
                  <small>{description}</small>
                </span>
                <ChevronRight size={15} />
              </button>
            ))}
          </div>
          <div className="config-note">
            <KeyRound size={17} />
            <strong>凭证安全保存</strong>
            <p>密钥不会回显。留空时保留已有配置。</p>
          </div>
        </aside>
        <form
          className="settings-form"
          onInvalidCapture={(e) => {
            const group = (e.target as HTMLElement)
              .closest("section")
              ?.getAttribute("data-group");
            if (group) setSection(group);
          }}
          onSubmit={(e) => { e.preventDefault(); void editor.save(); }}
          aria-busy={busy}
        >
          <fieldset className="settings-fields" disabled={!canEdit}>
          <section
            className="panel settings-section"
            data-group="model"
            hidden={section !== "model"}
          >
            <div className="settings-description">
              <span className="section-icon">
                <Bot size={22} />
              </span>
              <h2>模型服务</h2>
              <p>使用支持工具调用的 OpenAI 兼容接口。密钥留空保留原值。</p>
            </div>
            <div className="fields">
              <label>
                模型 API 地址
                <input
                  required
                  type="url"
                  value={s.openai.url}
                  onChange={(e) =>
                    setSettings({
                      ...s,
                      openai: { ...s.openai, url: e.target.value },
                    })
                  }
                />
              </label>
              <label>
                模型名称
                <input
                  required
                  value={s.openai.model}
                  onChange={(e) =>
                    setSettings({
                      ...s,
                      openai: { ...s.openai, model: e.target.value },
                      react: { ...s.react, model: "" },
                    })
                  }
                />
              </label>
              <label>
                API Key <small>{s.has_api_key ? "已设置" : "未设置"}</small>
                <input
                  type="password"
                  autoComplete="new-password"
                  placeholder="留空保留现有密钥"
                  value={s.openai.api_key}
                  onChange={(e) =>
                    setSettings({
                      ...s,
                      openai: { ...s.openai, api_key: e.target.value },
                    })
                  }
                />
              </label>
              <div className="field-row">
                <label>
                  温度
                  <input
                    type="number"
                    min="0"
                    max="2"
                    step="0.1"
                    required
                    value={s.react.temperature}
                    onChange={(e) =>
                      setSettings({
                        ...s,
                        react: {
                          ...s.react,
                          temperature: Number(e.target.value),
                        },
                      })
                    }
                  />
                </label>
                <label>
                  模型轮次上限
                  <input
                    type="number"
                    min="2"
                    max="100"
                    required
                    value={s.react.max_steps}
                    onChange={(e) =>
                      setSettings({
                        ...s,
                        react: {
                          ...s.react,
                          max_steps: Number(e.target.value),
                        },
                      })
                    }
                  />
                </label>
              </div>
              <label>独立复核模型（可选）<input value={s.verification_model||""} maxLength={128} placeholder="留空使用主审模型" onChange={e=>setSettings({...s,verification_model:e.target.value})}/><small>使用当前模型 API 和凭据。选择该接口支持的其他模型可增加复核多样性；独立复核仍需人工确认。</small></label>
              <ModelBudgetSettingsFields value={s.model_budget || {max_tokens:0,input_price_per_million:0,output_price_per_million:0,currency:"",verification_pricing_configured:false,verification_input_price_per_million:0,verification_output_price_per_million:0}} onChange={model_budget=>setSettings({...s,model_budget})} />
            </div>
          </section>
          <section
            className="panel settings-section"
            data-group="integration"
            hidden={section !== "integration"}
          >
            <div className="settings-description">
              <span className="section-icon">
                <GitBranch size={22} />
              </span>
              <h2>GitLab 集成</h2>
              <p>配置团队仓库实例和 Webhook。访问凭证仅保存在服务端。</p>
            </div>
            <div className="fields">
              <label>
                GitLab 地址
                <input
                  required
                  type="url"
                  value={s.gitlab.url}
                  onChange={(e) =>
                    setSettings({
                      ...s,
                      gitlab: { ...s.gitlab, url: e.target.value },
                    })
                  }
                />
              </label>
              <label>
                GitLab Token{" "}
                <small>{s.has_gitlab_token ? "已设置" : "未设置"}</small>
                <input
                  type="password"
                  autoComplete="new-password"
                  value={s.gitlab.token}
                  placeholder="留空保留现有 Token"
                  onChange={(e) =>
                    setSettings({
                      ...s,
                      gitlab: { ...s.gitlab, token: e.target.value },
                    })
                  }
                />
              </label>
              <label>
                Webhook Token{" "}
                <small>{s.has_webhook_token ? "已设置" : "未设置"}</small>
                <input
                  type="password"
                  autoComplete="new-password"
                  value={s.webhook_token}
                  placeholder="GitLab Webhook 的 Secret Token"
                  onChange={(e) =>
                    setSettings({ ...s, webhook_token: e.target.value })
                  }
                />
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={s.enable_webhook}
                  onChange={(e) =>
                    setSettings({ ...s, enable_webhook: e.target.checked })
                  }
                />
                启用 Webhook
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={s.enable_polling}
                  onChange={(e) =>
                    setSettings({ ...s, enable_polling: e.target.checked })
                  }
                />
                启用定时轮询
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={s.enable_mr_comment}
                  onChange={(e) =>
                    setSettings({ ...s, enable_mr_comment: e.target.checked })
                  }
                />
                允许将结果评论到 MR
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={s.scan_existing_mrs}
                  onChange={(e) =>
                    setSettings({ ...s, scan_existing_mrs: e.target.checked })
                  }
                />
                轮询启动时审计存量 MR
              </label>
            </div>
          </section>
          <section
            className="panel settings-section"
            data-group="quotas"
            hidden={section !== "quotas"}
          >
            <div className="settings-description">
              <span className="section-icon">
                <Gauge size={22} />
              </span>
              <h2>审计配额</h2>
              <p>控制队列容量、项目与用户并发，以及滚动审计次数。</p>
            </div>
            <div className="fields">
              <AuditQuotaSettingsFields
                value={s.audit_quotas}
                onChange={(audit_quotas) => setSettings({ ...s, audit_quotas })}
              />
            </div>
          </section>
          <section
            className="panel settings-section"
            data-group="execution"
            hidden={section !== "execution"}
          >
            <div className="settings-description">
              <span className="section-icon">
                <SlidersHorizontal size={22} />
              </span>
              <h2>执行策略</h2>
              <p>控制并发与任务预算。排除文件会记录在覆盖说明中。</p>
            </div>
            <div className="fields">
              <div className="settings-option-card">
                <label className="check">
                  <input
                    type="checkbox"
                    checked={s.generate_sequence_diagrams}
                    onChange={(e) =>
                      setSettings({
                        ...s,
                        generate_sequence_diagrams: e.target.checked,
                      })
                    }
                  />
                  发现问题后生成时序图
                </label>
                <p className="muted">
                  额外调用当前模型生成链路，增加耗时和 token
                  用量。生成失败会保留审计发现。
                </p>
              </div>

              <div className="settings-option-card">
                <label className="check">
                  <input type="checkbox" checked={s.verify_findings} onChange={(e) => setSettings({ ...s, verify_findings: e.target.checked })} />
                  对发现和调查命题进行独立复核
                </label>
                <p className="muted">在新的上下文中重新读取固定源码，先复核发现，再用共享预算复核已收尾的调查命题。额外消耗模型调用；判断分歧、证据不足或未完成会记入覆盖说明，原发现与初审判断保留，不执行仓库代码。</p>
              </div>
              <GitAuditSettingsFields
                value={s.git_audit}
                onChange={(git_audit) => setSettings({ ...s, git_audit })}
              />
              <div className="field-row">
                <label>
                  Worker 数
                  <input
                    type="number"
                    min="1"
                    max="16"
                    required
                    value={s.audit_workers}
                    onChange={(e) =>
                      setSettings({
                        ...s,
                        audit_workers: Number(e.target.value),
                      })
                    }
                  />
                </label>
                <label>
                  任务超时（秒）
                  <input
                    type="number"
                    min="10"
                    max="3600"
                    required
                    value={s.audit_timeout_seconds}
                    onChange={(e) =>
                      setSettings({
                        ...s,
                        audit_timeout_seconds: Number(e.target.value),
                      })
                    }
                  />
                </label>
              </div>
              <label>
                公开访问地址（反向代理 HTTPS）
                <input
                  type="url"
                  placeholder="https://audit.example.com（可留空）"
                  value={s.public_url || ""}
                  onChange={(e) =>
                    setSettings({ ...s, public_url: e.target.value })
                  }
                />
              </label>
              <label>
                监听地址
                <input
                  required
                  value={s.listen}
                  onChange={(e) =>
                    setSettings({ ...s, listen: e.target.value })
                  }
                />
              </label>
              <label>
                可信反向代理（IP 或 CIDR，逗号分隔）
                <input
                  placeholder="例如 127.0.0.1, 10.0.1.0/24"
                  aria-describedby="trusted-proxy-help"
                  value={trustedProxyDraft}
                  onChange={(e) => {
                    setTrustedProxyDraft(e.target.value);
                    setSettings({
                      ...s,
                      trusted_proxies: e.target.value
                        .split(",")
                        .map((x) => x.trim())
                        .filter(Boolean),
                    });
                  }}
                />
                <small className="muted" id="trusted-proxy-help">
                  只填写实际代理地址。留空时忽略转发的客户端
                  IP；保存后需要重启服务。
                </small>
              </label>
              <label>
                排除扩展名（逗号分隔）
                <input
                  value={s.whitelist_extensions.join(", ")}
                  onChange={(e) =>
                    setSettings({
                      ...s,
                      whitelist_extensions: e.target.value
                        .split(",")
                        .map((x) => x.trim())
                        .filter(Boolean),
                    })
                  }
                />
              </label>
            </div>
          </section>
          <div className="settings-save">
            <span className={dirty ? "save-state dirty" : "save-state"}>
              <CheckCircle2 size={16} />
              {dirty ? "有未保存的更改" : "没有未保存的更改"}
              <small>保存到 config.yaml</small>
            </span>
            <button className="primary" disabled={!canSave}>
              <Save size={16} />
              {busy ? "保存中…" : "保存设置"}
            </button>
          </div>
          </fieldset>
        </form>
      </div>
    </>
  );
}
