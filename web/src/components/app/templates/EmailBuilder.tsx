// Drag-and-drop HTML email builder (GrapesJS with the newsletter preset).
// Loaded on demand; returns table-based HTML with CSS inlined, which is what
// mail clients render reliably.

import React from "react";
import grapesjs, { type Editor } from "grapesjs";
import newsletter from "grapesjs-preset-newsletter";
import "grapesjs/dist/css/grapes.min.css";
import { Loader2Icon, PaletteIcon, XIcon } from "lucide-react";
import { useConfirm } from "@/hooks/context/confirm";

const STARTER = `<table style="width:100%;max-width:600px;margin:0 auto;font-family:Arial,sans-serif" cellpadding="0" cellspacing="0">
  <tr><td style="padding:24px 24px 8px;font-size:22px;font-weight:bold;color:#0f172a">Hi {{.FirstName}},</td></tr>
  <tr><td style="padding:8px 24px 24px;font-size:15px;line-height:1.6;color:#334155">Drag blocks from the right to build your email. Merge fields like {{.Company}} work anywhere.</td></tr>
</table>`;

export default function EmailBuilder({ initialHTML, onSave, onClose }: { initialHTML: string; onSave: (html: string) => void; onClose: () => void }) {
    const confirm = useConfirm();
    const container = React.useRef<HTMLDivElement>(null);
    const editorRef = React.useRef<Editor | null>(null);
    const dirty = React.useRef(false);
    const [ready, setReady] = React.useState(false);

    React.useEffect(() => {
        if (!container.current) return;
        const editor = grapesjs.init({
            container: container.current,
            height: "100%",
            width: "auto",
            fromElement: false,
            storageManager: false,
            plugins: [newsletter],
            pluginsOpts: { [newsletter as unknown as string]: { inlineCss: true } },
            components: initialHTML.trim() || STARTER,
        });
        editor.on("load", () => setReady(true));
        editor.on("update", () => {
            dirty.current = true;
        });
        editorRef.current = editor;
        return () => {
            editor.destroy();
            editorRef.current = null;
        };
        // Initialised once per open; later prop changes must not reset the canvas.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const requestClose = React.useCallback(() => {
        if (dirty.current) confirm.show("Discard the changes you made in the builder?", onClose);
        else onClose();
    }, [confirm, onClose]);

    React.useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape" || document.querySelector("[role='alertdialog']")) return;
            requestClose();
        };
        document.addEventListener("keydown", onKey);
        return () => document.removeEventListener("keydown", onKey);
    }, [requestClose]);

    function save() {
        const editor = editorRef.current;
        if (!editor) return;
        const inlined = editor.runCommand("gjs-get-inlined-html") as string | undefined;
        onSave(inlined && inlined.trim() ? inlined : `<style>${editor.getCss() ?? ""}</style>${editor.getHtml()}`);
    }

    return (
        <div className="fixed inset-0 z-[120] bg-white flex flex-col" role="dialog" aria-label="Email builder">
            <div className="h-12 px-4 border-b border-slate-200 flex items-center gap-2.5 shrink-0">
                <div className="size-5 rounded bg-slate-100 text-slate-600 flex items-center justify-center">
                    <PaletteIcon className="w-3 h-3" />
                </div>
                <span className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium">Design</span>
                <div className="h-4 w-px bg-slate-200" />
                <span className="text-[12.5px] text-slate-900 font-medium">HTML email</span>
                {!ready && <Loader2Icon className="w-3.5 h-3.5 animate-spin text-slate-400" />}
                <span className="hidden md:inline text-[11px] text-slate-500 ml-2">Merge fields such as {"{{.FirstName}}"} are kept as typed.</span>
                <div className="ml-auto flex items-center gap-1.5">
                    <button type="button" onClick={requestClose} className="h-7 px-2.5 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700">
                        Cancel
                    </button>
                    <button type="button" onClick={save} disabled={!ready} className="h-7 px-3 rounded-md bg-sky-600 hover:bg-sky-700 disabled:bg-slate-200 text-white text-[12px] font-medium">
                        Use this design
                    </button>
                    <button type="button" onClick={requestClose} aria-label="Close" className="size-7 rounded-md text-slate-500 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center">
                        <XIcon className="w-3.5 h-3.5" />
                    </button>
                </div>
            </div>
            <div className="flex-1 min-h-0">
                <div ref={container} className="h-full" />
            </div>
        </div>
    );
}
