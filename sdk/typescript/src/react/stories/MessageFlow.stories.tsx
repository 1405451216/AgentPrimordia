/**
 * MessageFlow.stories.tsx — MessageFlow 组件 Storybook stories。
 */

import type { Meta, StoryObj } from '@storybook/react';
import { MessageFlow } from '../collaboration/MessageFlow.js';

const meta = {
  title: 'Collaboration/MessageFlow',
  component: MessageFlow,
  tags: ['autodocs'],
} satisfies Meta<typeof MessageFlow>;

export default meta;
type Story = StoryObj<typeof meta>;

const sampleAgents = [
  { id: 'a1', name: 'Research Agent', status: 'working' as const },
  { id: 'a2', name: 'Writer Agent', status: 'idle' as const },
];

export const Empty: Story = {
  args: { messages: [], agents: sampleAgents },
};

export const WithMessages: Story = {
  args: {
    agents: sampleAgents,
    messages: [
      { id: 'm1', from: 'user', content: 'Please analyze this topic' },
      { id: 'm2', from: 'a1', to: 'a2', content: 'I found some interesting data', kind: 'message' },
      { id: 'm3', from: 'a1', content: 'search("topic")', kind: 'tool_call' },
      { id: 'm4', from: 'a2', content: 'Search returned 42 results', kind: 'tool_result' },
      { id: 'm5', from: 'a2', to: 'user', content: 'Here is the summary based on the research.' },
    ],
  },
};

export const WithError: Story = {
  args: {
    agents: sampleAgents,
    messages: [
      { id: 'm1', from: 'a1', content: 'Processing...' },
      { id: 'm2', from: 'system', content: 'Connection timeout', kind: 'error' },
    ],
  },
};
