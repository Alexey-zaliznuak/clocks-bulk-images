import { createContext, useContext, useState, type ReactNode } from "react";
import { api, getToken, setToken, clearToken } from "./api";

interface AuthContextValue {
  isAuthed: boolean;
  login: (login: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthed, setIsAuthed] = useState<boolean>(!!getToken());

  async function login(loginValue: string, password: string) {
    const { token } = await api.login(loginValue, password);
    setToken(token);
    setIsAuthed(true);
  }

  function logout() {
    clearToken();
    setIsAuthed(false);
  }

  return (
    <AuthContext.Provider value={{ isAuthed, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
