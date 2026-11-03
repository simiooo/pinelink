import * as React from "react"
import { cn } from "cn"
import { Progress as ProgressPrimitive } from "radix-ui"

function Progress({
  className,
  value,
  indicatorRef,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root> & { indicatorRef?: React.Ref<HTMLDivElement> }) {
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      value={value}
      className={cn(
        "relative h-[3px] w-full overflow-hidden rounded-full bg-[#E5E5E5]",
        className
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        ref={indicatorRef}
        data-slot="progress-indicator"
        className="h-full w-full flex-1 bg-primary"
        style={{ transform: `translateX(-${100 - (value || 0)}%)` }}
      />
    </ProgressPrimitive.Root>
  )
}

export { Progress }
