import { t, useLocale } from "../i18n";
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
  useLocale();
  return (
    <section>
      <h1>{t("Full YAML")}</h1>
      <p className="intro">
        {t(
          "Shares the same draft as the form. Form edits preserve unknown fields, variable references, and YAML comments.",
        )}
      </p>
      <Textarea
        className="yaml-editor"
        aria-label={t("Full YAML configuration")}
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
            setNotice(t("Configuration is valid."));
          })
        }
      >
        {t("Validate configuration")}
      </Button>
    </section>
  );
}
