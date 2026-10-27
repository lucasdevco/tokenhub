import { Button } from "../components/ui/button";
import { Textarea } from "../components/ui/textarea";
import { Alert, AlertDescription } from "../components/ui/alert";
import { request as api } from "./settings-api";

type Props = {
  draft: string;
  setDraft: (draft: string) => void;
  yamlError: string;
  locked: boolean;
  setNotice: (notice: string) => void;
  perform: (action: () => Promise<void>) => Promise<void>;
};

export default function YamlSettings({
  draft,
  setDraft,
  yamlError,
  locked,
  setNotice,
  perform,
}: Props) {
  return (
    <section>
      <h1>完整 YAML</h1>
      <p className="intro">
        与表单共用同一份草稿。表单编辑保留未知字段、变量引用和 YAML 注释。
      </p>
      <Textarea
        className="yaml-editor"
        aria-label="完整 YAML 配置"
        spellCheck={false}
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value);
          setNotice("");
        }}
      />
      {yamlError && (
        <Alert variant="destructive" className="message error" role="alert">
          <AlertDescription>{yamlError}</AlertDescription>
        </Alert>
      )}
      <Button
        variant="outline"
        size="sm"
        disabled={locked}
        onClick={() =>
          perform(async () => {
            await api("/api/config/validate", { yaml: draft });
            setNotice("配置校验通过。");
          })
        }
      >
        校验配置
      </Button>
    </section>
  );
}
