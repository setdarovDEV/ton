import { Link } from 'react-router-dom';
import { EmptyState, Button } from '@/components/ui';

export function NotFoundPage() {
  return (
    <EmptyState
      title="Page not found"
      action={
        <Link to="/">
          <Button variant="secondary">Back to Explore</Button>
        </Link>
      }
    />
  );
}
