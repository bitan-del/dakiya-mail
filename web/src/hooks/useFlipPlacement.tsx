import React from "react";

/**
 * Decides whether a popup should render above or below its trigger
 * based on available room inside the nearest scrolling/clipping
 * ancestor. Use when an absolute-positioned dropdown lives inside a
 * container with overflow:auto|hidden|scroll (modal body, slide-over,
 * etc.) where a too-tall dropdown would get clipped.
 *
 * Measures on open. Cheap — getBoundingClientRect once per ancestor.
 */
export default function useFlipPlacement(
    triggerRef: React.RefObject<HTMLElement | null>,
    open: boolean,
    estimatedHeight: number,
): "top" | "bottom" {
    const [placement, setPlacement] = React.useState<"top" | "bottom">("bottom");

    React.useLayoutEffect(() => {
        if (!open) return;
        const trigger = triggerRef.current;
        if (!trigger) return;

        const rect = trigger.getBoundingClientRect();
        let clipBottom = window.innerHeight;
        let clipTop = 0;
        let el: HTMLElement | null = trigger.parentElement;
        while (el) {
            const s = getComputedStyle(el);
            const overflow = `${s.overflow} ${s.overflowY} ${s.overflowX}`;
            if (/(auto|scroll|hidden)/.test(overflow)) {
                const ar = el.getBoundingClientRect();
                if (ar.bottom < clipBottom) clipBottom = ar.bottom;
                if (ar.top > clipTop) clipTop = ar.top;
            }
            el = el.parentElement;
        }

        const spaceBelow = clipBottom - rect.bottom;
        const spaceAbove = rect.top - clipTop;
        if (spaceBelow < estimatedHeight && spaceAbove > spaceBelow) {
            setPlacement("top");
        } else {
            setPlacement("bottom");
        }
    }, [open, triggerRef, estimatedHeight]);

    return placement;
}
