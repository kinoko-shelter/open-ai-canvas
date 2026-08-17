import ReactMarkdown from "react-markdown";
import type { ReactNode } from "react";

type AIMessageMarkdownProps = {
    children: string;
    isStreaming?: boolean;
    className?: string;
};

const markdownComponents = {
    h1: ({ children }: { children?: ReactNode }) => <h2 className="ai-message-markdown-heading ai-message-markdown-heading-1">{children}</h2>,
    h2: ({ children }: { children?: ReactNode }) => <h3 className="ai-message-markdown-heading ai-message-markdown-heading-2">{children}</h3>,
    h3: ({ children }: { children?: ReactNode }) => <h4 className="ai-message-markdown-heading ai-message-markdown-heading-3">{children}</h4>,
    h4: ({ children }: { children?: ReactNode }) => <h5 className="ai-message-markdown-heading ai-message-markdown-heading-4">{children}</h5>,
    p: ({ children }: { children?: ReactNode }) => <p className="ai-message-markdown-paragraph">{children}</p>,
    ul: ({ children }: { children?: ReactNode }) => <ul className="ai-message-markdown-list ai-message-markdown-list-unordered">{children}</ul>,
    ol: ({ children }: { children?: ReactNode }) => <ol className="ai-message-markdown-list ai-message-markdown-list-ordered">{children}</ol>,
    li: ({ children }: { children?: ReactNode }) => <li className="ai-message-markdown-list-item">{children}</li>,
    blockquote: ({ children }: { children?: ReactNode }) => <blockquote className="ai-message-markdown-blockquote">{children}</blockquote>,
    pre: ({ children }: { children?: ReactNode }) => <pre className="ai-message-markdown-pre">{children}</pre>,
    code: ({ children }: { children?: ReactNode }) => <code className="ai-message-markdown-code">{children}</code>,
    a: ({ children, href }: { children?: ReactNode; href?: string }) => (
        <a className="ai-message-markdown-link" href={href} target="_blank" rel="noreferrer">
            {children}
        </a>
    ),
    hr: () => <hr className="ai-message-markdown-rule" />,
    table: ({ children }: { children?: ReactNode }) => (
        <div className="ai-message-markdown-table-wrap">
            <table className="ai-message-markdown-table">{children}</table>
        </div>
    ),
    th: ({ children }: { children?: ReactNode }) => <th className="ai-message-markdown-table-cell ai-message-markdown-table-header">{children}</th>,
    td: ({ children }: { children?: ReactNode }) => <td className="ai-message-markdown-table-cell">{children}</td>,
};

export function AIMessageMarkdown({ children, isStreaming = false, className = "" }: AIMessageMarkdownProps) {
    if (!children.trim()) return null;
    const markdown = completeStreamingCodeFence(children, isStreaming);
    return (
        <div className={`ai-message-markdown${isStreaming ? " is-streaming" : ""}${className ? ` ${className}` : ""}`} aria-live={isStreaming ? "polite" : undefined}>
            <ReactMarkdown skipHtml components={markdownComponents}>
                {markdown}
            </ReactMarkdown>
        </div>
    );
}

function completeStreamingCodeFence(value: string, isStreaming: boolean) {
    if (!isStreaming) return value;
    const fenceCount = value.match(/(^|\n)```/g)?.length || 0;
    return fenceCount % 2 === 0 ? value : `${value}\n\`\`\``;
}
