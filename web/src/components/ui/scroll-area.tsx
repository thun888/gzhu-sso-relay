import {
  OverlayScrollbarsComponent,
  type OverlayScrollbarsComponentProps,
} from "overlayscrollbars-react"

export const scrollOptions = {
  scrollbars: {
    theme: "os-theme-relay",
    autoHide: "never",
    autoHideDelay: 700,
    clickScroll: true,
  },
} as const

export function ScrollArea({ options, ...props }: OverlayScrollbarsComponentProps) {
  return <OverlayScrollbarsComponent options={options ?? scrollOptions} {...props} />
}
