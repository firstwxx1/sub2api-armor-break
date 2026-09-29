import { apiClient } from '../client'

export type ArmorBreakMode = 'replace' | 'prepend'

export interface PersonaInfo {
  name: string
  size: number
  updated_at: string
}

export interface ArmorBreakState {
  enabled: boolean
  mode: ArmorBreakMode
  persona: string
  personas: PersonaInfo[]
  persona_dir: string
  load_error?: string
}

export interface ArmorBreakUpdateRequest {
  enabled?: boolean
  mode?: ArmorBreakMode
  persona?: string
}

export interface PersonaContent {
  name: string
  size: number
  content: string
}

export async function getArmorBreakState(): Promise<ArmorBreakState> {
  const { data } = await apiClient.get<ArmorBreakState>('/admin/armor-break')
  return data
}

export async function updateArmorBreakState(req: ArmorBreakUpdateRequest): Promise<ArmorBreakState> {
  const { data } = await apiClient.put<ArmorBreakState>('/admin/armor-break', req)
  return data
}

export async function getPersonaContent(name: string): Promise<PersonaContent> {
  const { data } = await apiClient.get<PersonaContent>(`/admin/armor-break/personas/${encodeURIComponent(name)}`)
  return data
}

export async function putPersona(name: string, content: string): Promise<PersonaInfo> {
  const { data } = await apiClient.put<PersonaInfo>(`/admin/armor-break/personas/${encodeURIComponent(name)}`, { content })
  return data
}

export async function deletePersona(name: string): Promise<void> {
  await apiClient.delete(`/admin/armor-break/personas/${encodeURIComponent(name)}`)
}
