import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  MessageSquare, 
  Send, 
  User, 
  Clock, 
  ShieldCheck, 
  Pin, 
  Check, 
  Search 
} from 'lucide-react';
import { ChatMessage } from '../../types';

interface LiveChatViewProps {
  initialTargetUserId?: string;
}

export const LiveChatView: React.FC<LiveChatViewProps> = ({ initialTargetUserId }) => {
  const { 
    chatMessages, 
    sendMessage, 
    currentUser, 
    currentRole, 
    users 
  } = useVeylixStore();

  const [inputText, setInputText] = useState('');
  const [selectedChannel, setSelectedChannel] = useState<string>('channel_general');

  // Channels list:
  // General community, plus direct chats
  const channels = [
    { id: 'channel_general', name: 'Global Operations & Support', role: 'public', unread: 0 },
    { id: 'channel_alex', name: 'Alex Rivers (Client Support)', role: 'client', unread: 0 },
    { id: 'channel_sarah', name: 'Sarah Chen (Order Verification)', role: 'client', unread: 1 }
  ];

  const filteredMessages = chatMessages.filter(m => 
    selectedChannel === 'channel_general' ? (m.channelId === 'general' || m.channelId === 'channel_general') : m.channelId === selectedChannel
  );

  const handleSend = (e: React.FormEvent) => {
    e.preventDefault();
    if (!inputText.trim()) return;

    sendMessage(currentUser.id, inputText.trim(), selectedChannel);
    setInputText('');
  };

  return (
    <div className="h-[calc(100vh-8rem)] flex flex-col md:flex-row gap-4">
      {/* Channels / Conversations Sidebar */}
      <div className="w-full md:w-72 bg-neutral-900 border border-neutral-800 rounded-2xl p-4 flex flex-col justify-between shrink-0">
        <div className="space-y-3">
          <div className="flex items-center space-x-2 pb-3 border-b border-neutral-800">
            <MessageSquare className="w-4 h-4 text-emerald-400" />
            <h2 className="text-sm font-bold text-white">Support Channels</h2>
          </div>

          <div className="space-y-1">
            {channels.map(chan => {
              const isSelected = selectedChannel === chan.id;
              return (
                <button
                  key={chan.id}
                  onClick={() => setSelectedChannel(chan.id)}
                  className={`w-full text-left p-3 rounded-xl transition flex items-center justify-between text-xs ${
                    isSelected
                      ? 'bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 font-semibold'
                      : 'hover:bg-neutral-800/60 text-neutral-400 hover:text-white'
                  }`}
                >
                  <div className="truncate">
                    <p className="truncate text-neutral-200">{chan.name}</p>
                    <p className="text-[10px] text-neutral-500 capitalize">{chan.role} channel</p>
                  </div>
                  {chan.unread > 0 && (
                    <span className="w-4 h-4 rounded-full bg-emerald-500 text-neutral-950 font-bold text-[10px] flex items-center justify-center shrink-0">
                      {chan.unread}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        </div>

        {/* Quick Help Tip */}
        <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl text-[11px] text-neutral-400 space-y-1">
          <p className="font-semibold text-white">Direct Communication</p>
          <p>Real-time encrypted dispatch between Clients, Resellers, and Infrastructure Ops.</p>
        </div>
      </div>

      {/* Main Conversation Window */}
      <div className="flex-1 bg-neutral-900 border border-neutral-800 rounded-2xl flex flex-col justify-between overflow-hidden shadow-xl">
        {/* Chat Header */}
        <div className="px-6 py-4 border-b border-neutral-800 bg-neutral-950/50 flex items-center justify-between">
          <div>
            <h3 className="font-bold text-white text-sm">
              {channels.find(c => c.id === selectedChannel)?.name || 'Direct Channel'}
            </h3>
            <p className="text-[11px] text-neutral-400 flex items-center space-x-1.5 mt-0.5">
              <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
              <span>Encrypted Veylix Internal Channel • Participant Verified</span>
            </p>
          </div>
        </div>

        {/* Message Stream */}
        <div className="flex-1 p-6 overflow-y-auto space-y-4">
          {filteredMessages.map(msg => {
            const isMe = msg.senderId === currentUser.id;

            return (
              <div 
                key={msg.id} 
                className={`flex flex-col ${isMe ? 'items-end' : 'items-start'} space-y-1`}
              >
                <div className="flex items-center space-x-1.5 text-[10px] text-neutral-500">
                  <span className="font-semibold text-neutral-300">{msg.senderName}</span>
                  <span className="uppercase font-mono text-[9px] px-1 py-0.2 bg-neutral-800 rounded text-neutral-400">
                    {msg.senderRole}
                  </span>
                  <span>•</span>
                  <span>{new Date(msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                </div>

                <div className={`p-3.5 rounded-2xl max-w-md text-xs leading-relaxed ${
                  isMe 
                    ? 'bg-emerald-500 text-neutral-950 font-medium rounded-tr-sm shadow-md' 
                    : 'bg-neutral-950 border border-neutral-800 text-neutral-200 rounded-tl-sm'
                }`}>
                  {msg.text}
                </div>
              </div>
            );
          })}
        </div>

        {/* Message Input Box */}
        <form onSubmit={handleSend} className="p-4 border-t border-neutral-800 bg-neutral-950/60 flex items-center space-x-2">
          <input
            type="text"
            value={inputText}
            onChange={(e) => setInputText(e.target.value)}
            placeholder="Type message to client or support..."
            className="flex-1 px-4 py-2.5 bg-neutral-900 border border-neutral-800 rounded-xl text-xs text-white placeholder-neutral-500 focus:outline-none focus:border-emerald-500/50"
          />
          <button
            type="submit"
            className="px-4 py-2.5 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
          >
            <Send className="w-3.5 h-3.5" />
            <span>Send</span>
          </button>
        </form>
      </div>
    </div>
  );
};
