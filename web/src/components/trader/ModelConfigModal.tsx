import React, { useState, useEffect } from 'react'
import { Trash2, Brain, ExternalLink } from 'lucide-react'
import type { AIModel, ReasoningEffort } from '../../types'
import { REASONING_EFFORTS } from '../../types'
import type { Language } from '../../i18n/translations'
import { t } from '../../i18n/translations'
import { getModelIcon } from '../common/ModelIcons'
import { ModelStepIndicator } from './ModelStepIndicator'
import { ModelCard } from './ModelCard'
import {
  AI_PROVIDER_CONFIG,
  getShortName,
} from './model-constants'

const EFFORT_LABEL_KEYS: Record<ReasoningEffort, string> = {
  low: 'effortLow',
  medium: 'effortMedium',
  high: 'effortHigh',
  xhigh: 'effortXHigh',
  max: 'effortMax',
}

interface ModelConfigModalProps {
  allModels: AIModel[]
  configuredModels: AIModel[]
  editingModelId: string | null
  onSave: (
    modelId: string,
    apiKey: string,
    baseUrl?: string,
    modelName?: string,
    reasoningEffort?: ReasoningEffort
  ) => void
  onDelete: (modelId: string) => void
  onClose: () => void
  language: Language
}

export function ModelConfigModal({
  allModels,
  configuredModels,
  editingModelId,
  onSave,
  onDelete,
  onClose,
  language,
}: ModelConfigModalProps) {
  const [currentStep, setCurrentStep] = useState(editingModelId ? 1 : 0)
  const [selectedModelId, setSelectedModelId] = useState(editingModelId || '')
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [modelName, setModelName] = useState('')
  const [reasoningEffort, setReasoningEffort] = useState<ReasoningEffort>('low')

  // Always prefer allModels (supportedModels) for provider/id lookup;
  // fall back to configuredModels for edit mode details (apiKey etc.)
  const configuredModel = configuredModels?.find((m) => m.id === selectedModelId)
  const selectedModel =
    allModels?.find((m) => m.id === selectedModelId) || configuredModel
  // An existing model already has a stored key; a blank input means "keep it".
  const hasStoredKey = !!(editingModelId && configuredModel?.has_api_key)

  useEffect(() => {
    if (editingModelId) {
      // allModels is the static provider catalog and carries no per-user
      // settings; read the configured row for the saved effort.
      const configured = configuredModels?.find((m) => m.id === editingModelId)
      setApiKey(selectedModel?.apiKey || '')
      setBaseUrl(selectedModel?.customApiUrl || '')
      setModelName(selectedModel?.customModelName || '')
      setReasoningEffort(
        configured?.reasoningEffort || selectedModel?.reasoningEffort || 'low'
      )
    }
  }, [editingModelId, selectedModel, configuredModels])

  const handleSelectModel = (modelId: string) => {
    setSelectedModelId(modelId)
    setCurrentStep(1)
  }

  const handleBack = () => {
    if (editingModelId) {
      onClose()
    } else {
      setCurrentStep(0)
      setSelectedModelId('')
    }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedModelId || (!apiKey.trim() && !hasStoredKey)) return
    onSave(selectedModelId, apiKey.trim(), baseUrl.trim() || undefined, modelName.trim() || undefined, reasoningEffort)
  }

  const availableModels = allModels || []
  const configuredIds = new Set(configuredModels?.map(m => m.id) || [])
  const stepLabels = [t('modelConfig.selectModel', language), t('modelConfig.configureApi', language)]

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4 overflow-y-auto backdrop-blur-sm">
      <div
        className="rounded-2xl w-full max-w-[52rem] relative my-8 shadow-2xl"
        style={{
          background: 'linear-gradient(180deg, #1E2329 0%, #181A20 100%)',
          maxHeight: 'calc(100vh - 4rem)',
        }}
      >
        {/* Header */}
        <div className="flex items-center justify-between p-6 pb-2">
          <div className="flex items-center gap-3">
            {currentStep > 0 && !editingModelId && (
              <button
                type="button"
                onClick={handleBack}
                className="p-2 rounded-lg hover:bg-white/10 transition-colors"
              >
                <svg
                  className="w-5 h-5"
                  style={{ color: '#848E9C' }}
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M15 19l-7-7 7-7"
                  />
                </svg>
              </button>
            )}
            <h3 className="text-xl font-bold" style={{ color: '#EAECEF' }}>
              {editingModelId
                ? t('editAIModel', language)
                : t('addAIModel', language)}
            </h3>
          </div>
          <div className="flex items-center gap-2">
            {editingModelId && (
              <button
                type="button"
                onClick={() => onDelete(editingModelId)}
                className="p-2 rounded-lg hover:bg-red-500/20 transition-colors"
                style={{ color: '#F6465D' }}
              >
                <Trash2 className="w-4 h-4" />
              </button>
            )}
            <button
              type="button"
              onClick={onClose}
              className="p-2 rounded-lg hover:bg-white/10 transition-colors"
              style={{ color: '#848E9C' }}
            >
              ✕
            </button>
          </div>
        </div>

        {/* Step Indicator */}
        {!editingModelId && (
          <div className="px-6">
            <ModelStepIndicator currentStep={currentStep} labels={stepLabels} />
          </div>
        )}

        {/* Content */}
        <div
          className="px-6 pb-6 overflow-y-auto"
          style={{ maxHeight: 'calc(100vh - 16rem)' }}
        >
          {/* Step 0: Select Model */}
          {currentStep === 0 && !editingModelId && (
            <ModelSelectionStep
              availableModels={availableModels}
              configuredIds={configuredIds}
              selectedModelId={selectedModelId}
              onSelectModel={handleSelectModel}
              language={language}
            />
          )}

          {/* Step 1: Configure */}
          {(currentStep === 1 || editingModelId) && selectedModel && (
            <StandardProviderConfigForm
              selectedModel={selectedModel}
              apiKey={apiKey}
              baseUrl={baseUrl}
              modelName={modelName}
              reasoningEffort={reasoningEffort}
              hasStoredKey={hasStoredKey}
              editingModelId={editingModelId}
              onApiKeyChange={setApiKey}
              onBaseUrlChange={setBaseUrl}
              onModelNameChange={setModelName}
              onReasoningEffortChange={setReasoningEffort}
              onBack={handleBack}
              onSubmit={handleSubmit}
              language={language}
            />
          )}
        </div>
      </div>
    </div>
  )
}

// --- Sub-components for ModelConfigModal ---

function ModelSelectionStep({
  availableModels,
  configuredIds,
  selectedModelId,
  onSelectModel,
  language,
}: {
  availableModels: AIModel[]
  configuredIds: Set<string>
  selectedModelId: string
  onSelectModel: (modelId: string) => void
  language: Language
}) {
  return (
    <div className="space-y-4">
      <div className="text-sm font-semibold" style={{ color: '#EAECEF' }}>
        {t('modelConfig.chooseProvider', language)}
      </div>

      <div className="grid grid-cols-3 sm:grid-cols-4 gap-3">
        {availableModels.map((model) => (
          <ModelCard
            key={model.id}
            model={model}
            selected={selectedModelId === model.id}
            onClick={() => onSelectModel(model.id)}
            configured={configuredIds.has(model.id)}
          />
        ))}
      </div>
      <div className="text-xs text-center pt-2" style={{ color: '#848E9C' }}>
        {t('modelConfig.modelsConfigured', language)}
      </div>
    </div>
  )
}

function StandardProviderConfigForm({
  selectedModel,
  apiKey,
  baseUrl,
  modelName,
  reasoningEffort,
  hasStoredKey,
  editingModelId,
  onApiKeyChange,
  onBaseUrlChange,
  onModelNameChange,
  onReasoningEffortChange,
  onBack,
  onSubmit,
  language,
}: {
  selectedModel: AIModel
  apiKey: string
  baseUrl: string
  modelName: string
  reasoningEffort: ReasoningEffort
  hasStoredKey: boolean
  editingModelId: string | null
  onApiKeyChange: (value: string) => void
  onBaseUrlChange: (value: string) => void
  onModelNameChange: (value: string) => void
  onReasoningEffortChange: (value: ReasoningEffort) => void
  onBack: () => void
  onSubmit: (e: React.FormEvent) => void
  language: Language
}) {
  return (
    <form onSubmit={onSubmit} className="space-y-5">
      {/* Selected Model Header */}
      <div
        className="p-4 rounded-xl flex items-center gap-4"
        style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
      >
        <div className="w-12 h-12 rounded-xl flex items-center justify-center bg-black border border-white/10">
          {getModelIcon(selectedModel.provider || selectedModel.id, {
            width: 32,
            height: 32,
          }) || (
            <span className="text-lg font-bold" style={{ color: '#A78BFA' }}>
              {selectedModel.name[0]}
            </span>
          )}
        </div>
        <div className="flex-1">
          <div className="font-semibold text-lg" style={{ color: '#EAECEF' }}>
            {getShortName(selectedModel.name)}
          </div>
          <div className="text-xs" style={{ color: '#848E9C' }}>
            {selectedModel.provider} •{' '}
            {AI_PROVIDER_CONFIG[selectedModel.provider]?.defaultModel ||
              selectedModel.id}
          </div>
        </div>
        {AI_PROVIDER_CONFIG[selectedModel.provider] && (
          <a
            href={AI_PROVIDER_CONFIG[selectedModel.provider].apiUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 px-4 py-2 rounded-lg transition-all hover:scale-105"
            style={{
              background: 'rgba(139, 92, 246, 0.1)',
              border: '1px solid rgba(139, 92, 246, 0.3)',
            }}
          >
            <ExternalLink className="w-4 h-4" style={{ color: '#A78BFA' }} />
            <span className="text-sm font-medium" style={{ color: '#A78BFA' }}>
              {t('modelConfig.getApiKey', language)}
            </span>
          </a>
        )}
      </div>

      {/* Kimi Warning */}
      {selectedModel.provider === 'kimi' && (
        <div
          className="p-4 rounded-xl"
          style={{
            background: 'rgba(246, 70, 93, 0.1)',
            border: '1px solid rgba(246, 70, 93, 0.3)',
          }}
        >
          <div className="flex items-start gap-2">
            <span style={{ fontSize: '16px' }}>⚠️</span>
            <div className="text-sm" style={{ color: '#F6465D' }}>
              {t('kimiApiNote', language)}
            </div>
          </div>
        </div>
      )}

      {/* API Key */}
      {editingModelId && selectedModel && 'has_api_key' in selectedModel && (
        <div
          className="p-3 rounded-xl text-xs"
          style={{ background: 'rgba(14, 203, 129, 0.08)', border: '1px solid rgba(14, 203, 129, 0.2)', color: '#9FE8C5' }}
        >
          当前模型密钥状态：{selectedModel.has_api_key ? '已配置 API Key' : '未配置 API Key'}
        </div>
      )}

      <div className="space-y-2">
        <label
          className="flex items-center gap-2 text-sm font-semibold"
          style={{ color: '#EAECEF' }}
        >
          <svg
            className="w-4 h-4"
            style={{ color: '#A78BFA' }}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
            />
          </svg>
          API Key{!hasStoredKey && ' *'}
        </label>
        <input
          type="password"
          value={apiKey}
          onChange={(e) => onApiKeyChange(e.target.value)}
          placeholder={
            hasStoredKey
              ? '已保存，如需更换请重新输入'
              : t('enterAPIKey', language)
          }
          className="w-full px-4 py-3 rounded-xl"
          style={{
            background: '#0B0E11',
            border: '1px solid #2B3139',
            color: '#EAECEF',
          }}
          required={!hasStoredKey}
        />
      </div>

      {/* Custom Base URL */}
      <div className="space-y-2">
        <label className="flex items-center gap-2 text-sm font-semibold" style={{ color: '#EAECEF' }}>
          <svg className="w-4 h-4" style={{ color: '#A78BFA' }} fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
          </svg>
          {t('customBaseURL', language)}
        </label>
        <input
          type="url"
          value={baseUrl}
          onChange={(e) => onBaseUrlChange(e.target.value)}
          placeholder={t('customBaseURLPlaceholder', language)}
          className="w-full px-4 py-3 rounded-xl"
          style={{ background: '#0B0E11', border: '1px solid #2B3139', color: '#EAECEF' }}
        />
        <div className="text-xs" style={{ color: '#848E9C' }}>
          {t('leaveBlankForDefault', language)}
        </div>
      </div>

      {/* Custom Model Name */}
      <div className="space-y-2">
        <label className="flex items-center gap-2 text-sm font-semibold" style={{ color: '#EAECEF' }}>
          <svg className="w-4 h-4" style={{ color: '#A78BFA' }} fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
          </svg>
          {t('customModelName', language)}
        </label>
        <input
          type="text"
          value={modelName}
          onChange={(e) => onModelNameChange(e.target.value)}
          placeholder={t('customModelNamePlaceholder', language)}
          className="w-full px-4 py-3 rounded-xl"
          style={{ background: '#0B0E11', border: '1px solid #2B3139', color: '#EAECEF' }}
        />
        <div className="text-xs" style={{ color: '#848E9C' }}>
          {t('leaveBlankForDefaultModel', language)}
        </div>
      </div>

      {/* Reasoning Effort */}
      <div className="space-y-2">
        <label className="flex items-center gap-2 text-sm font-semibold" style={{ color: '#EAECEF' }}>
          <Brain className="w-4 h-4" style={{ color: '#A78BFA' }} />
          {t('reasoningEffort', language)}
        </label>
        <div className="grid grid-cols-5 gap-2" role="group" aria-label={t('reasoningEffort', language)}>
          {REASONING_EFFORTS.map((effort) => {
            const selected = reasoningEffort === effort
            return (
              <button
                key={effort}
                type="button"
                aria-pressed={selected}
                onClick={() => onReasoningEffortChange(effort)}
                className="px-2 py-2 rounded-lg text-xs font-semibold transition-all"
                style={{
                  background: selected ? 'rgba(139, 92, 246, 0.18)' : '#0B0E11',
                  border: `1px solid ${selected ? '#8B5CF6' : '#2B3139'}`,
                  color: selected ? '#C4B5FD' : '#848E9C',
                }}
              >
                {t(EFFORT_LABEL_KEYS[effort], language)}
              </button>
            )
          })}
        </div>
        <div className="text-xs" style={{ color: '#848E9C' }}>
          {t('reasoningEffortHint', language)}
        </div>
      </div>

      {/* Info Box */}
      <div
        className="p-4 rounded-xl"
        style={{
          background: 'rgba(139, 92, 246, 0.1)',
          border: '1px solid rgba(139, 92, 246, 0.2)',
        }}
      >
        <div
          className="text-sm font-semibold mb-2 flex items-center gap-2"
          style={{ color: '#A78BFA' }}
        >
          <Brain className="w-4 h-4" />
          {t('information', language)}
        </div>
        <div className="text-xs space-y-1" style={{ color: '#848E9C' }}>
          <div>• {t('modelConfigInfo1', language)}</div>
          <div>• {t('modelConfigInfo2', language)}</div>
          <div>• {t('modelConfigInfo3', language)}</div>
        </div>
      </div>

      {/* Buttons */}
      <div className="flex gap-3 pt-4">
        <button
          type="button"
          onClick={onBack}
          className="flex-1 px-4 py-3 rounded-xl text-sm font-semibold transition-all hover:bg-white/5"
          style={{ background: '#2B3139', color: '#848E9C' }}
        >
          {editingModelId
            ? t('cancel', language)
            : t('modelConfig.back', language)}
        </button>
        <button
          type="submit"
          disabled={!selectedModel || (!apiKey.trim() && !hasStoredKey)}
          className="flex-1 flex items-center justify-center gap-2 px-4 py-3 rounded-xl text-sm font-bold transition-all hover:scale-[1.02] disabled:opacity-50 disabled:cursor-not-allowed"
          style={{ background: '#8B5CF6', color: '#fff' }}
        >
          {t('saveConfig', language)}
          <svg
            className="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M14 5l7 7m0 0l-7 7m7-7H3"
            />
          </svg>
        </button>
      </div>
    </form>
  )
}
