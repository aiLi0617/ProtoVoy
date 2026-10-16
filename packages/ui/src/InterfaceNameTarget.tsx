import { createContext, useContext, type ReactNode } from "react";

const InterfaceNameTargetContext = createContext<HTMLElement | null>(null);

export function InterfaceNameTargetProvider({ target, children }: { target: HTMLElement | null; children: ReactNode }) {
  return <InterfaceNameTargetContext.Provider value={target}>{children}</InterfaceNameTargetContext.Provider>;
}

export function useInterfaceNameTarget() {
  return useContext(InterfaceNameTargetContext);
}
