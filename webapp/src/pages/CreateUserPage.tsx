import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useCreateUser } from '../services/services';
import { UserCreateRequest } from '@/apiclient';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Input } from '../components/ui/Input';

export function CreateUserPage() {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<UserCreateRequest>({
    username: '',
    email: '',
    password: '',
  });
  const [message, setMessage] = useState('');
  const [messageType, setMessageType] = useState<'success' | 'error' | ''>('');
  const createUserMutation = useCreateUser();

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setFormData((prevFormData: UserCreateRequest) => ({
      ...prevFormData,
      [name]: value,
    }));
  };

  const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage('');
    setMessageType('');
    createUserMutation.mutate(formData, {
      onSuccess: (createdUser) => {
        if (createdUser) {
          setMessage(`User created successfully! ID: ${createdUser.id}, Username: ${createdUser.username}. Redirecting...`);
          setMessageType('success');
          setFormData({ username: '', email: '', password: '' });
          setTimeout(() => {
            navigate(`/users/${createdUser.id}`);
          }, 1500);
        }
      },
      onError: (err: any) => {
        setMessage(err.message || 'An unexpected error occurred. Please try again.');
        setMessageType('error');
      },
    });
  };

  let messageClasses = 'p-3 mb-5 rounded-md text-sm';
  if (messageType === 'success') {
    messageClasses += ' bg-success/10 text-success border border-success/20';
  } else if (messageType === 'error') {
    messageClasses += ' bg-danger/10 text-danger border border-danger/20';
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">User Management</h1>
        <p className="mt-1 text-sm text-muted-fg">Create a new user account</p>
      </div>

      <Card>
        <CardHeader>
          <h2 className="text-base font-semibold">Create New User</h2>
        </CardHeader>
        <CardContent>
          {message && messageType && (
            <div className={messageClasses} role="alert">
              {message}
            </div>
          )}
          <form onSubmit={handleSubmit}>
            <div>
              <label htmlFor="username" className="mb-2 block text-sm font-medium text-muted-fg">
                Username:
              </label>
              <Input
                type="text"
                id="username"
                name="username"
                value={formData.username}
                onChange={handleChange}
                required
                disabled={createUserMutation.isPending}
                className="mb-5"
              />
            </div>
            <div>
              <label htmlFor="email" className="mb-2 block text-sm font-medium text-muted-fg">
                Email:
              </label>
              <Input
                type="email"
                id="email"
                name="email"
                value={formData.email}
                onChange={handleChange}
                required
                disabled={createUserMutation.isPending}
                className="mb-5"
              />
            </div>
            <div>
              <label htmlFor="password" className="mb-2 block text-sm font-medium text-muted-fg">
                Password:
              </label>
              <Input
                type="password"
                id="password"
                name="password"
                value={formData.password}
                onChange={handleChange}
                required
                disabled={createUserMutation.isPending}
                className="mb-5"
              />
            </div>
            <Button type="submit" disabled={createUserMutation.isPending} className="w-full">
              {createUserMutation.isPending ? 'Creating…' : 'Create User'}
            </Button>
          </form>
          <div className="mt-8 border-t border-border pt-6 text-center text-sm">
            <Link to="/" className="tg-link mr-5">
              &larr; Home
            </Link>
            <Link to="/users" className="tg-link">
              View All Users
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
